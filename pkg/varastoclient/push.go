package varastoclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/djherbis/times"
	"github.com/function61/gokit/ezhttp"
	"github.com/function61/varasto/pkg/stateresolver"
	"github.com/function61/varasto/pkg/varastotypes"
	"github.com/function61/varasto/pkg/varastoutils"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	mebibyte = 1024 * 1024
	blobSize = 4 * mebibyte
)

func computeChangeset(wd *workdirLocation) (*varastotypes.CollectionChangeset, error) {
	parentState, err := stateresolver.ComputeStateAt(wd.manifest.Collection, wd.manifest.Collection.Head)
	if err != nil {
		return nil, err
	}

	// deleted during directory scan. what's left over is what are missing w.r.t. parent state
	filesMissing := parentState.Files()
	filesAtParent := parentState.Files() // this will not be mutated

	created := []varastotypes.File{}
	updated := []varastotypes.File{}

	errWalk := filepath.Walk(wd.path, func(path string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err // stop if encountering Walk() errors
		}

		if !fileInfo.IsDir() { // don't handle directories
			if fileInfo.Name() == localStatefile { // skip .varasto files
				return nil
			}

			// this returns \ on Windows, but we'll need to normalize to slashes for interop
			relativePath, errRel := filepath.Rel(wd.path, path)
			if errRel != nil {
				return errRel
			}
			relativePath = backslashesToForwardSlashes(relativePath)

			// ok if key missing
			delete(filesMissing, relativePath)

			if before, existedBefore := filesAtParent[relativePath]; existedBefore {
				definitelyChanged := before.Size != fileInfo.Size()
				maybeChanged := !before.Modified.Equal(fileInfo.ModTime())

				if definitelyChanged || maybeChanged {
					fil, err := analyzeFileForChanges(wd.Join(relativePath), relativePath, fileInfo)
					if err != nil {
						return err
					}

					// TODO: allow commit to change metadata like modification time or
					// execute bit?
					if before.Sha256 != fil.Sha256 {
						updated = append(updated, *fil)
					}
				}
			} else {
				fil, err := analyzeFileForChanges(wd.Join(relativePath), relativePath, fileInfo)
				if err != nil {
					return err
				}

				created = append(created, *fil)
			}
		}

		return nil
	})
	if errWalk != nil {
		return nil, errWalk
	}

	deleted := []string{}
	for missing, _ := range filesMissing {
		deleted = append(deleted, missing)
	}
	sort.Strings(deleted)

	ch := varastotypes.NewChangeset(
		varastoutils.NewCollectionChangesetId(),
		wd.manifest.ChangesetId,
		time.Now(),
		created,
		updated,
		deleted)

	return &ch, nil
}

// returns ErrChunkMetadataNotFound if blob is not found
func blobExists(blobRef varastotypes.BlobRef, clientConfig ClientConfig) (bool, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), ezhttp.DefaultTimeout10s)
	defer cancel()

	// do a HEAD request to see if the blob exists
	resp, err := ezhttp.Head(
		ctx,
		clientConfig.ApiPath("/api/blobs/"+blobRef.AsHex()),
		ezhttp.AuthBearer(clientConfig.AuthToken))

	if err != nil && resp != nil && resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	if err != nil {
		return false, err // an actual error
	}

	return true, nil
}

func analyzeFileForChanges(absolutePath string, relativePath string, fileInfo os.FileInfo) (*varastotypes.File, error) {
	// https://unix.stackexchange.com/questions/2802/what-is-the-difference-between-modify-and-change-in-stat-command-context
	allTimes := times.Get(fileInfo)

	maybeCreationTime := fileInfo.ModTime()
	if allTimes.HasBirthTime() {
		maybeCreationTime = allTimes.BirthTime()
	}

	bfile := &varastotypes.File{
		Path:     relativePath,
		Created:  maybeCreationTime,
		Modified: fileInfo.ModTime(),
		Size:     fileInfo.Size(),
		Sha256:   "",         // will be computed later in this method
		BlobRefs: []string{}, // will be computed later in this method
	}

	file, err := os.Open(absolutePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	pos := int64(0)

	fullContentHash := sha256.New()

	for {
		if _, err := file.Seek(pos, io.SeekStart); err != nil {
			return nil, err
		}

		chunk, errRead := ioutil.ReadAll(io.LimitReader(file, blobSize))
		if errRead != nil {
			return nil, errRead
		}

		pos += blobSize

		if len(chunk) == 0 {
			// should only happen if file size is exact multiple of blobSize
			break
		}

		if _, err := fullContentHash.Write(chunk); err != nil {
			return nil, err
		}

		fileSha256Bytes := sha256.Sum256(chunk)

		blobRef, err := varastotypes.BlobRefFromHex(hex.EncodeToString(fileSha256Bytes[:]))
		if err != nil {
			return nil, err
		}

		bfile.BlobRefs = append(bfile.BlobRefs, blobRef.AsHex())

		if int64(len(chunk)) < blobSize {
			break
		}
	}

	bfile.Sha256 = fmt.Sprintf("%x", fullContentHash.Sum(nil))

	return bfile, nil
}

func uploadChunks(path string, bfile varastotypes.File, collection varastotypes.Collection, clientConfig ClientConfig) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	for blobIdx, brHex := range bfile.BlobRefs {
		blobRef, err := varastotypes.BlobRefFromHex(brHex)
		if err != nil {
			return err
		}

		// just check if the chunk exists already
		blobAlreadyExists, err := blobExists(*blobRef, clientConfig)
		if err != nil {
			return err
		}

		if blobAlreadyExists {
			log.Printf("Deduplicated chunk %s", blobRef.AsHex())
			continue
		}

		if _, err := file.Seek(int64(blobIdx*blobSize), io.SeekStart); err != nil {
			return err
		}

		chunk := io.LimitReader(file, blobSize)

		// 10 seconds can be too fast waiting for HDD to spin up + blob write
		ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)

		if _, err := ezhttp.Post(
			ctx,
			clientConfig.ApiPath("/api/blobs/"+blobRef.AsHex()+"?collection="+collection.ID),
			ezhttp.AuthBearer(clientConfig.AuthToken),
			ezhttp.SendBody(varastoutils.BlobHashVerifier(chunk, *blobRef), "application/octet-stream")); err != nil {
			cancel()
			return err
		}

		cancel()
	}

	return nil
}

func uploadChangeset(changeset varastotypes.CollectionChangeset, collection varastotypes.Collection, clientConfig ClientConfig) (*varastotypes.Collection, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), ezhttp.DefaultTimeout10s)
	defer cancel()

	updatedCollection := &varastotypes.Collection{}
	_, err := ezhttp.Post(
		ctx,
		clientConfig.ApiPath("/api/collections/"+collection.ID+"/changesets"),
		ezhttp.AuthBearer(clientConfig.AuthToken),
		ezhttp.SendJson(&changeset),
		ezhttp.RespondsJson(&updatedCollection, false))

	return updatedCollection, err
}

func pushOne(collectionId string, path string) error {
	clientConfig, err := ReadConfig()
	if err != nil {
		return err
	}

	coll, err := FetchCollectionMetadata(*clientConfig, collectionId)
	if err != nil {
		return err
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	absolutePath := filepath.Join(wd, path)

	fileInfo, err := os.Stat(absolutePath)
	if err != nil {
		return err
	}

	file, err := analyzeFileForChanges(absolutePath, path, fileInfo)
	if err != nil {
		return err
	}

	if err := uploadChunks(path, *file, *coll, *clientConfig); err != nil {
		return err
	}

	changeset := varastotypes.NewChangeset(
		varastoutils.NewCollectionChangesetId(),
		coll.Head,
		time.Now(),
		[]varastotypes.File{*file},
		[]varastotypes.File{},
		[]string{})

	_, err = uploadChangeset(changeset, *coll, *clientConfig)
	return err
}

func push(wd *workdirLocation) error {
	ch, err := computeChangeset(wd)
	if err != nil {
		return err
	}

	if !ch.AnyChanges() {
		log.Println("No files changed")
		return nil
	}

	for _, created := range ch.FilesCreated {
		if err := uploadChunks(wd.Join(created.Path), created, wd.manifest.Collection, wd.clientConfig); err != nil {
			return err
		}
	}

	for _, updated := range ch.FilesUpdated {
		if err := uploadChunks(wd.Join(updated.Path), updated, wd.manifest.Collection, wd.clientConfig); err != nil {
			return err
		}
	}

	updatedCollection, err := uploadChangeset(*ch, wd.manifest.Collection, wd.clientConfig)
	if err != nil {
		return err
	}

	wd.manifest.ChangesetId = updatedCollection.Head
	wd.manifest.Collection = *updatedCollection

	return wd.SaveToDisk()
}

func backslashesToForwardSlashes(in string) string {
	return strings.Replace(in, `\`, "/", -1)
}
