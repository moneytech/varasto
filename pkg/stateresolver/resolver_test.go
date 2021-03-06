package stateresolver

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/function61/gokit/assert"
	"github.com/function61/varasto/pkg/stotypes"
	"github.com/function61/varasto/pkg/stoutils"
)

func TestComputeStateAt(t *testing.T) {
	// new empty collection
	coll := stotypes.Collection{
		Head:       stotypes.NoParentId,
		Changesets: []stotypes.CollectionChangeset{},
	}

	assert.Assert(t, len(coll.Changesets) == 0)
	assert.EqualString(t, dumpState(coll, coll.Head), `
`)

	coll = pushChangeset(coll, stotypes.NoParentId, creates("a.txt", 11), creates("b.txt", 22))

	assert.Assert(t, len(coll.Changesets) == 1)
	assert.EqualString(t, dumpState(coll, coll.Head), `a.txt (size 11)
b.txt (size 22)
`)

	coll = pushChangeset(coll, coll.Head, creates("c.txt", 33), updates("b.txt", 44))

	assert.Assert(t, len(coll.Changesets) == 2)
	assert.EqualString(t, dumpState(coll, coll.Head), `a.txt (size 11)
b.txt (size 44)
c.txt (size 33)
`)

	coll = pushChangeset(coll, coll.Head, deletes("a.txt"))

	assert.Assert(t, len(coll.Changesets) == 3)
	assert.EqualString(t, dumpState(coll, coll.Head), `b.txt (size 44)
c.txt (size 33)
`)

	// now go back in time to 2nd changeset
	assert.EqualString(t, dumpState(coll, coll.Changesets[1].ID), `a.txt (size 11)
b.txt (size 44)
c.txt (size 33)
`)
}

// test helpers

func pushChangeset(coll stotypes.Collection, parentId string, mutations ...chMutFn) stotypes.Collection {
	changeset := stotypes.NewChangeset(
		stoutils.NewCollectionChangesetId(),
		parentId,
		time.Now(),
		nil,
		nil,
		nil)

	for _, mutate := range mutations {
		mutate(&changeset)
	}

	coll.Changesets = append(coll.Changesets, changeset)
	coll.Head = changeset.ID

	return coll
}

func dumpState(coll stotypes.Collection, revId string) string {
	state, err := ComputeStateAt(coll, revId)
	if err != nil {
		panic(err)
	}

	asList := []string{}

	for _, file := range state.files {
		asList = append(asList, fmt.Sprintf("%s (size %d)", file.Path, file.Size))
	}

	sort.Strings(asList)

	return strings.Join(asList, "\n") + "\n"
}

type chMutFn func(ch *stotypes.CollectionChangeset)

func creates(name string, size int64) chMutFn {
	return func(ch *stotypes.CollectionChangeset) {
		ch.FilesCreated = append(ch.FilesCreated, stotypes.File{
			Path: name,
			Size: size,
		})
	}
}

func updates(name string, size int64) chMutFn {
	return func(ch *stotypes.CollectionChangeset) {
		ch.FilesUpdated = append(ch.FilesUpdated, stotypes.File{
			Path: name,
			Size: size,
		})
	}
}

func deletes(name string) chMutFn {
	return func(ch *stotypes.CollectionChangeset) {
		ch.FilesDeleted = append(ch.FilesDeleted, name)
	}
}
