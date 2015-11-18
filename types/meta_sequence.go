package types

import (
	"crypto/sha1"

	"github.com/attic-labs/noms/Godeps/_workspace/src/github.com/attic-labs/buzhash"
	"github.com/attic-labs/noms/chunks"
	"github.com/attic-labs/noms/d"
	"github.com/attic-labs/noms/ref"
)

const (
	objectWindowSize = 8 * sha1.Size
	objectPattern    = uint32(1<<6 - 1) // Average size of 64 elements
)

// metaSequence is a logical abstraction, but has no concrete "base" implementation. A Meta Sequence is a non-leaf (internal) node of a "probably" tree, which results from the chunking of an ordered or unordered sequence of values.

type metaSequence interface {
	tupleAt(idx int) metaTuple
	lastTuple() metaTuple
	tupleCount() int
	Ref() ref.Ref
}

type metaTuple struct {
	ref   ref.Ref
	value Value
}

func (mt metaTuple) uint64Value() uint64 {
	return uint64(mt.value.(UInt64))
}

type metaSequenceData []metaTuple

type metaSequenceObject struct {
	tuples metaSequenceData
	t      Type
}

func (ms metaSequenceObject) tupleAt(idx int) metaTuple {
	return ms.tuples[idx]
}

func (ms metaSequenceObject) tupleCount() int {
	return len(ms.tuples)
}

func (ms metaSequenceObject) lastTuple() metaTuple {
	return ms.tuples[len(ms.tuples)-1]
}

func (ms metaSequenceObject) ChildValues() []Value {
	leafType := ms.t.Desc.(CompoundDesc).ElemTypes[0]
	refOfLeafType := MakeCompoundType(RefKind, leafType)
	res := make([]Value, len(ms.tuples))
	for i, t := range ms.tuples {
		res[i] = refFromType(t.ref, refOfLeafType)
	}
	return res
}

func (ms metaSequenceObject) Chunks() (chunks []ref.Ref) {
	for _, tuple := range ms.tuples {
		chunks = append(chunks, tuple.ref)
	}
	return
}

func (ms metaSequenceObject) Type() Type {
	return ms.t
}

type metaBuilderFunc func(tuples metaSequenceData, t Type, cs chunks.ChunkSource) Value
type metaReaderFunc func(v Value) metaSequenceData

type metaSequenceFuncs struct {
	builder metaBuilderFunc
	reader  metaReaderFunc
}

var (
	metaFuncMap map[NomsKind]metaSequenceFuncs = map[NomsKind]metaSequenceFuncs{}
)

func registerMetaValue(k NomsKind, bf metaBuilderFunc, rf metaReaderFunc) {
	metaFuncMap[k] = metaSequenceFuncs{bf, rf}
}

func newMetaSequenceFromData(tuples metaSequenceData, t Type, cs chunks.ChunkSource) Value {
	concreteType := t.Desc.(CompoundDesc).ElemTypes[0]

	if s, ok := metaFuncMap[concreteType.Kind()]; ok {
		return s.builder(tuples, t, cs)
	}

	panic("not reachable")
}

func getDataFromMetaSequence(v Value) metaSequenceData {
	concreteType := v.Type().Desc.(CompoundDesc).ElemTypes[0]

	if s, ok := metaFuncMap[concreteType.Kind()]; ok {
		return s.reader(v)
	}

	panic("not reachable")
}

func metaSequenceIsBoundaryFn() isBoundaryFn {
	h := buzhash.NewBuzHash(objectWindowSize)

	return func(item sequenceItem) bool {
		mt, ok := item.(metaTuple)
		d.Chk.True(ok)
		digest := mt.ref.Digest()
		_, err := h.Write(digest[:])
		d.Chk.NoError(err)
		return h.Sum32()&objectPattern == objectPattern
	}
}

func newMetaSequenceChunkFn(t Type, cs chunks.ChunkStore) makeChunkFn {
	return func(items []sequenceItem) (sequenceItem, interface{}) {
		tuples := make(metaSequenceData, len(items))
		offsetSum := uint64(0)

		for i, v := range items {
			mt, ok := v.(metaTuple)
			d.Chk.True(ok)
			offsetSum += mt.uint64Value()
			mt.value = UInt64(offsetSum)
			tuples[i] = mt
		}

		meta := newMetaSequenceFromData(tuples, t, cs)
		ref := WriteValue(meta, cs)
		return metaTuple{ref, UInt64(offsetSum)}, meta
	}
}
