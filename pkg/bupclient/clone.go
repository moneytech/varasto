package bupclient

import (
	"context"
	"errors"
	"github.com/function61/bup/pkg/buptypes"
	"github.com/function61/bup/pkg/buputils"
	"github.com/function61/bup/pkg/stateresolver"
	"github.com/function61/gokit/ezhttp"
	"github.com/function61/gokit/fileexists"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func clone(collectionId string, revisionId string, parentDir string, dirName string) error {
	clientConfig, err := readConfig()
	if err != nil {
		return err
	}

	collection, err := fetchCollectionMetadata(*clientConfig, collectionId)
	if err != nil {
		return err
	}

	if dirName == "" {
		dirName = collection.Name
	}

	return cloneCollection(filepath.Join(parentDir, dirName), revisionId, collection)
}

// used both by collection create and collection download
func cloneCollection(path string, revisionId string, collection *buptypes.Collection) error {
	// init this in "hack mode" (i.e. statefile not being read to memory). as soon as we
	// manage to write the statefile to disk, use normal procedure to init wd
	halfBakedWd := &workdirLocation{
		path: path,
	}

	dirAlreadyExists, err := fileexists.Exists(halfBakedWd.Join("/"))
	if err != nil {
		return err
	}

	if dirAlreadyExists {
		return errors.New("dir-to-clone-to already exists!")
	}

	if err := os.Mkdir(halfBakedWd.Join("/"), 0700); err != nil {
		return err
	}

	if revisionId == "" {
		revisionId = collection.Head
	}

	halfBakedWd.manifest = &BupManifest{
		ChangesetId: revisionId,
		Collection:  *collection,
	}

	if err := halfBakedWd.SaveToDisk(); err != nil {
		return err
	}

	// now that properly initialized halfBakedWd was saved to disk (= bootstrapped),
	// reload it back from disk in a normal fashion
	wd, err := NewWorkdirLocation(halfBakedWd.path)
	if err != nil {
		return err
	}

	state, err := stateresolver.ComputeStateAt(*collection, wd.manifest.ChangesetId)
	if err != nil {
		return err
	}

	for _, file := range state.Files() {
		if err := cloneOneFile(wd, file); err != nil {
			return err
		}
	}

	return nil
}

func cloneOneFile(wd *workdirLocation, file buptypes.File) error {
	log.Printf("Downloading %s", file.Path)

	filename := wd.Join(file.Path)
	filenameTemp := filename + ".temp"

	// does not error if already exists
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		return err
	}

	fileHandle, err := os.Create(filenameTemp)
	if err != nil {
		return err
	}
	defer fileHandle.Close()

	for _, chunkDigest := range file.BlobRefs {
		blobRef, err := buptypes.BlobRefFromHex(chunkDigest)
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.TODO(), 15*time.Second)
		defer cancel()

		chunkDataRes, err := ezhttp.Send(
			ctx,
			http.MethodGet,
			wd.clientConfig.ApiPath("/blobs/"+blobRef.AsHex()),
			ezhttp.AuthBearer(wd.clientConfig.AuthToken))
		if err != nil {
			return err
		}
		defer chunkDataRes.Body.Close()

		verifiedBody := buputils.BlobHashVerifier(chunkDataRes.Body, *blobRef)

		if _, err := io.Copy(fileHandle, verifiedBody); err != nil {
			return err
		}
	}

	fileHandle.Close() // even though we have the defer above - we probably need this for Chtimes()

	if err := os.Chtimes(filenameTemp, time.Now(), file.Modified); err != nil {
		return err
	}

	return os.Rename(filenameTemp, filename)
}

func fetchCollectionMetadata(clientConfig ClientConfig, id string) (*buptypes.Collection, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), ezhttp.DefaultTimeout10s)
	defer cancel()

	collection := &buptypes.Collection{}
	_, err := ezhttp.Send(
		ctx,
		http.MethodGet,
		clientConfig.ApiPath("/collections/"+id),
		ezhttp.AuthBearer(clientConfig.AuthToken),
		ezhttp.RespondsJson(collection, false))

	return collection, err
}
