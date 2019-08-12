package blorm

import (
	"bytes"
	"go.etcd.io/bbolt"
)

/*	types of indices
	================

	setIndex
	--------
	(pending_replication, "_", id) = nil

	simpleIndex
	-----------
	(by_parent, parentId, id) = nil
*/

var (
	StartFromFirst = []byte("")
)

// fully qualified index reference, including the index name
type qualifiedIndexRef struct {
	indexName string // looks like directories:by_parent
	valAndId  unqualifiedIndexRef
}

type unqualifiedIndexRef struct {
	val []byte // for setIndex this is always " "
	id  []byte // primary key of record the index entry refers to
}

func (i *qualifiedIndexRef) Equals(other *qualifiedIndexRef) bool {
	return i.indexName == other.indexName &&
		bytes.Equal(i.valAndId.val, other.valAndId.val) &&
		bytes.Equal(i.valAndId.id, other.valAndId.id)
}

func mkIndexRef(indexName string, val []byte, id []byte) qualifiedIndexRef {
	return qualifiedIndexRef{indexName, unqualifiedIndexRef{val, id}}
}

func mkUqIndexRef(val []byte, id []byte) unqualifiedIndexRef {
	return unqualifiedIndexRef{val, id}
}

type Index interface {
	// only for our internal use
	extractIndexRefs(record interface{}) []qualifiedIndexRef
}

type setIndexApi interface {
	// return StopIteration if you want to stop mid-iteration (nil error will be returned by Query() )
	Query(start []byte, fn func(id []byte) error, tx *bolt.Tx) error
	Index
}

type byValueIndexApi interface {
	// return StopIteration if you want to stop mid-iteration (nil error will be returned by Query() )
	Query(val []byte, start []byte, fn func(id []byte) error, tx *bolt.Tx) error
	Index
}

type setIndex struct {
	repo            *simpleRepository
	name            string // looks like <repoBucketName>:<indexName>
	memberEvaluator func(record interface{}) bool
}

func (s *setIndex) extractIndexRefs(record interface{}) []qualifiedIndexRef {
	if s.memberEvaluator(record) {
		return []qualifiedIndexRef{
			{
				indexName: s.name,
				valAndId:  mkUqIndexRef([]byte(" "), s.repo.idExtractor(record)),
			},
		}
	}

	return []qualifiedIndexRef{}
}

func (s *setIndex) Query(start []byte, fn func(id []byte) error, tx *bolt.Tx) error {
	// " " is required because empty key is not supported
	return indexQueryShared(s.name, []byte(" "), start, fn, tx)
}

func NewSetIndex(name string, repo *simpleRepository, memberEvaluator func(record interface{}) bool) setIndexApi {
	idx := &setIndex{repo, string(repo.bucketName) + ":" + name, memberEvaluator}

	repo.indices = append(repo.indices, idx)

	return idx
}

type byValueIndex struct {
	repo            *simpleRepository
	name            string // looks like <repoBucketName>:<indexName>
	memberEvaluator func(record interface{}, push func(val []byte))
}

func (b *byValueIndex) extractIndexRefs(record interface{}) []qualifiedIndexRef {
	qualifiedRefs := []qualifiedIndexRef{}
	b.memberEvaluator(record, func(val []byte) {
		if len(val) == 0 {
			panic("cannot index by empty value")
		}
		qualifiedRefs = append(qualifiedRefs, mkIndexRef(b.name, val, b.repo.idExtractor(record)))
	})

	return qualifiedRefs
}

func (b *byValueIndex) Query(value []byte, start []byte, fn func(id []byte) error, tx *bolt.Tx) error {
	return indexQueryShared(b.name, value, start, fn, tx)
}

// used both by byValueIndex and by setIndex
func indexQueryShared(indexName string, value []byte, start []byte, fn func(id []byte) error, tx *bolt.Tx) error {
	// the nil part is not used by indexBucketRefForQuery()
	bucket := indexBucketRefForQuery(mkIndexRef(indexName, value, nil), tx)
	if bucket == nil { // index doesn't exist => not matching entries
		return nil
	}

	idx := bucket.Cursor()

	var key []byte
	if bytes.Equal(start, StartFromFirst) {
		key, _ = idx.First()
	} else {
		key, _ = idx.Seek(start)
	}

	for ; key != nil; key, _ = idx.Next() {
		if err := fn(key); err != nil {
			if err == StopIteration {
				return nil
			} else {
				return err
			}
		}
	}

	return nil
}

func NewValueIndex(name string, repo *simpleRepository, memberEvaluator func(record interface{}, push func(val []byte))) byValueIndexApi {
	idx := &byValueIndex{repo, string(repo.bucketName) + ":" + name, memberEvaluator}

	repo.indices = append(repo.indices, idx)

	return idx
}

func indexRefExistsIn(ir qualifiedIndexRef, coll []qualifiedIndexRef) bool {
	for _, other := range coll {
		if ir.Equals(&other) {
			return true
		}
	}

	return false
}

func indexBucketRefForQuery(ref qualifiedIndexRef, tx *bolt.Tx) *bolt.Bucket {
	// directories:by_parent
	lvl1 := tx.Bucket([]byte(ref.indexName))
	if lvl1 == nil {
		return nil
	}

	return lvl1.Bucket(ref.valAndId.val)
}

func indexBucketRefForWrite(ref qualifiedIndexRef, tx *bolt.Tx) *bolt.Bucket {
	// directories:by_parent
	lvl1, err := tx.CreateBucketIfNotExists([]byte(ref.indexName))
	if err != nil {
		panic(err)
	}

	lvl2, err := lvl1.CreateBucketIfNotExists(ref.valAndId.val)
	if err != nil {
		panic(err)
	}

	return lvl2
}