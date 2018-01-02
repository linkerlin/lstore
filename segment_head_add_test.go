package lstore

import (
	"testing"
	"github.com/stretchr/testify/require"
	"github.com/v2pro/plz/countlog"
	"context"
	"github.com/esdb/biter"
	"fmt"
)

func intEntry(values ...int64) *Entry {
	return &Entry{EntryType: EntryTypeData, IntValues: values}
}

func blobEntry(values ...Blob) *Entry {
	return &Entry{EntryType: EntryTypeData, BlobValues: values}
}

func intBlobEntry(intValue int64, blobValue Blob) *Entry {
	return &Entry{EntryType: EntryTypeData, IntValues: []int64{intValue}, BlobValues: []Blob{blobValue}}
}

var ctx = countlog.Ctx(context.Background())

func fakeEditingHead() *editingHead {
	strategy := NewIndexingStrategy(IndexingStrategyConfig{
		BloomFilterIndexedBlobColumns: []int{0},
	})
	slotIndexManager := newSlotIndexManager(&slotIndexManagerConfig{
		IndexDirectory: "/tmp/store/index",
	}, strategy)
	headSegment, err := openHeadSegment(ctx, "/tmp/store/head.segment", slotIndexManager)
	if err != nil {
		panic(err)
	}
	return &editingHead{
		strategy:           strategy,
		headSegmentVersion: headSegment.headSegmentVersion,
		writeBlock: func(seq blockSeq, block *block) (blockSeq, blockSeq, error) {
			return seq, seq + 6, nil
		},
		slotIndexManager: slotIndexManager,
	}
}

func getLevel(editing *editingHead, level level) *slotIndex {
	slotIndex, err := editing.slotIndexManager.mapWritableSlotIndex(editing.levels[level], level)
	if err != nil {
		panic(err)
	}
	return slotIndex
}

func Test_add_first_block(t *testing.T) {
	should := require.New(t)
	editing := fakeEditingHead()
	editing.addBlock(ctx, newBlock(0, []*Entry{
		blobEntry("hello"),
	}))
	should.Equal(blockSeq(6), editing.tailBlockSeq)
	level0SlotIndex, err := editing.editLevel(level0)
	should.Nil(err)
	should.Equal([]uint64{0}, level0SlotIndex.children[:1])
	strategy := editing.strategy
	filterHello := strategy.NewBlobValueFilter(0, "hello")
	result := level0SlotIndex.searchSmall(filterHello)
	should.Equal(biter.SetBits[0], result)
	result = getLevel(editing, 1).searchMedium(filterHello)
	should.Equal(biter.SetBits[0], result)
	result = getLevel(editing, 2).searchLarge(filterHello)
	should.Equal(biter.SetBits[0], result)
	filter123 := strategy.NewBlobValueFilter(0, "123")
	result = getLevel(editing, 2).searchLarge(filter123)
	should.Equal(biter.Bits(0), result)
}

func Test_add_two_blocks(t *testing.T) {
	should := require.New(t)
	editing := fakeEditingHead()
	editing.addBlock(ctx, newBlock(0, []*Entry{
		blobEntry("hello"),
	}))
	editing.addBlock(ctx, newBlock(0, []*Entry{
		blobEntry("world"),
	}))
	should.Equal(blockSeq(12), editing.tailBlockSeq)
	level0SlotIndex := getLevel(editing, 0)
	should.Equal([]uint64{0, 6}, level0SlotIndex.children[:2])
	strategy := editing.strategy
	filterHello := strategy.NewBlobValueFilter(0, "hello")
	result := level0SlotIndex.searchSmall(filterHello)
	should.Equal(biter.SetBits[0], result)
	filterWorld := strategy.NewBlobValueFilter(0, "world")
	result = level0SlotIndex.searchSmall(filterWorld)
	should.Equal(biter.SetBits[1], result)
}

func Test_add_64_blocks(t *testing.T) {
	should := require.New(t)
	editing := fakeEditingHead()
	for i := 0; i < 64; i++ {
		editing.addBlock(ctx, newBlock(0, []*Entry{
			blobEntry(Blob(fmt.Sprintf("hello%d", i))),
		}))
	}
	should.Equal(blockSeq(6*64), editing.tailBlockSeq)
	should.Equal(slotIndexSeq(0), editing.tailSlotIndexSeq)
	level0SlotIndex := getLevel(editing, 0)
	strategy := editing.strategy
	result := level0SlotIndex.searchSmall(strategy.NewBlobValueFilter(0, "hello0"))
	should.Equal(biter.SetBits[0], result&biter.SetBits[0])
	result = level0SlotIndex.searchSmall(strategy.NewBlobValueFilter(0, "hello63"))
	should.Equal(biter.SetBits[63], result&biter.SetBits[63])
}

func Test_add_65_blocks(t *testing.T) {
	should := require.New(t)
	editing := fakeEditingHead()
	for i := 0; i < 65; i++ {
		editing.addBlock(ctx, newBlock(0, []*Entry{
			blobEntry(Blob(fmt.Sprintf("hello%d", i))),
		}))
	}
	should.Equal(blockSeq(6*65), editing.tailBlockSeq)
	should.Equal(slotIndexSeq(7), editing.tailSlotIndexSeq)
	should.Equal([]uint64{0}, getLevel(editing, 1).children[:1])
	level0SlotIndex := getLevel(editing, 0)
	strategy := editing.strategy
	result := level0SlotIndex.searchSmall(strategy.NewBlobValueFilter(0, "hello0"))
	should.Equal(biter.Bits(0), result, "level0 moved on, forget the old values")
	result = level0SlotIndex.searchSmall(strategy.NewBlobValueFilter(0, "hello64"))
	should.Equal(biter.SetBits[0], result)
	level1SlotIndex := getLevel(editing, 1)
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "hello0"))
	should.Equal(biter.SetBits[0], result, "level1 still remembers")
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "hello64"))
	should.Equal(biter.SetBits[1], result)
}

func Test_add_66_blocks(t *testing.T) {
	should := require.New(t)
	editing := fakeEditingHead()
	for i := 0; i < 66; i++ {
		editing.addBlock(ctx, newBlock(0, []*Entry{
			blobEntry(Blob(fmt.Sprintf("hello%d", i))),
		}))
	}
	should.Equal(blockSeq(6*66), editing.tailBlockSeq)
	should.Equal(slotIndexSeq(7), editing.tailSlotIndexSeq)
	should.Equal([]uint64{0}, getLevel(editing, 1).children[:1])
	level0SlotIndex := getLevel(editing, 0)
	strategy := editing.strategy
	result := level0SlotIndex.searchSmall(strategy.NewBlobValueFilter(0, "hello65"))
	should.Equal(biter.SetBits[1], result)
	level1SlotIndex := getLevel(editing, 1)
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "hello65"))
	should.Equal(biter.SetBits[1], result)
}

func Test_add_129_blocks(t *testing.T) {
	should := require.New(t)
	editing := fakeEditingHead()
	for i := 0; i < 129; i++ {
		editing.addBlock(ctx, newBlock(0, []*Entry{
			blobEntry(Blob(fmt.Sprintf("hello%d", i))),
		}))
	}
	should.Equal(blockSeq(6*129), editing.tailBlockSeq)
	should.Equal(slotIndexSeq(14), editing.tailSlotIndexSeq)
	should.Equal([]uint64{0, 7}, getLevel(editing, 1).children[:2])
	level0SlotIndex := getLevel(editing, 0)
	strategy := editing.strategy
	result := level0SlotIndex.searchSmall(strategy.NewBlobValueFilter(0, "hello128"))
	should.Equal(biter.SetBits[0], result)
	level1SlotIndex := getLevel(editing, 1)
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "hello63"))
	should.Equal(biter.SetBits[0], result)
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "hello64"))
	should.Equal(biter.SetBits[1], result)
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "hello127"))
	should.Equal(biter.SetBits[1], result)
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "hello128"))
	should.Equal(biter.SetBits[2], result)
}

func Test_add_64x64_blocks(t *testing.T) {
	blockLength = 2
	blockLengthInPowerOfTwo = 1
	should := require.New(t)
	editing := fakeEditingHead()
	for i := 0; i < 4096; i++ {
		editing.addBlock(ctx, newBlock(0, []*Entry{
			blobEntry(Blob(fmt.Sprintf("hello%d", i))),
		}))
	}
	should.Equal(blockSeq(6*4096), editing.tailBlockSeq)
	should.Equal(slotIndexSeq(7*63), editing.tailSlotIndexSeq)
	level0SlotIndex := getLevel(editing, 0)
	strategy := editing.strategy
	result := level0SlotIndex.searchSmall(strategy.NewBlobValueFilter(0, "hello4095"))
	should.Equal(biter.SetBits[63], result)
	level1SlotIndex := getLevel(editing, 1)
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "hello4095"))
	should.Equal(biter.SetBits[63], result)
}

func Test_add_64x64_plus_1_blocks(t *testing.T) {
	blockLength = 2
	blockLengthInPowerOfTwo = 1
	should := require.New(t)
	editing := fakeEditingHead()
	for i := 0; i < 4097; i++ {
		editing.addBlock(ctx, newBlock(0, []*Entry{
			blobEntry(Blob(fmt.Sprintf("hello%d", i))),
		}))
	}
	should.Equal(blockSeq(6*4097), editing.tailBlockSeq)
	should.Equal(slotIndexSeq(7*65), editing.tailSlotIndexSeq)
	level0SlotIndex := getLevel(editing, 0)
	strategy := editing.strategy
	result := level0SlotIndex.searchSmall(strategy.NewBlobValueFilter(0, "hello4096"))
	should.Equal(biter.SetBits[0], result)
	result = level0SlotIndex.searchSmall(strategy.NewBlobValueFilter(0, "hello4095"))
	should.Equal(biter.Bits(0), result, "level0 forget")
	level1SlotIndex := getLevel(editing, 1)
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "hello4096"))
	should.Equal(biter.SetBits[0], result)
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "hello4095"))
	should.Equal(biter.Bits(0), result, "level1 forget")
	level2SlotIndex := getLevel(editing, 2)
	result = level2SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "hello0"))
	should.Equal(biter.SetBits[0], result, "level2 still remembers")
	result = level2SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "hello4095"))
	should.Equal(biter.SetBits[0], result, "level2 still remembers")
	result = level2SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "hello4096"))
	should.Equal(biter.SetBits[1], result, "level2 still remembers")
}

func Test_add_64x64x64_blocks(t *testing.T) {
	blockLength = 2
	blockLengthInPowerOfTwo = 1
	should := require.New(t)
	editing := fakeEditingHead()
	for i := 0; i < 64*64*64; i++ {
		editing.addBlock(ctx, newBlock(0, []*Entry{
			blobEntry(Blob(fmt.Sprintf("hello%d", i))),
		}))
	}
	should.Equal(blockSeq(6*64*64*64), editing.tailBlockSeq)
	should.Equal(slotIndexSeq(7*4158), editing.tailSlotIndexSeq)
	strategy := editing.strategy
	level0SlotIndex := getLevel(editing, 0)
	result := level0SlotIndex.searchSmall(strategy.NewBlobValueFilter(0, "hello262143"))
	should.Equal(biter.SetBits[63], result)
	level1SlotIndex := getLevel(editing, 1)
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "hello262143"))
	should.Equal(biter.SetBits[63], result)
	level2SlotIndex := getLevel(editing, 2)
	result = level2SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "hello262143"))
	should.Equal(biter.SetBits[63], result)
}

func Test_add_64x64x64_plus_1_blocks(t *testing.T) {
	blockLength = 2
	blockLengthInPowerOfTwo = 1
	should := require.New(t)
	editing := fakeEditingHead()
	for i := 0; i < 64*64*64+1; i++ {
		editing.addBlock(ctx, newBlock(0, []*Entry{
			blobEntry(Blob(fmt.Sprintf("hello%d", i))),
		}))
	}
	should.Equal(blockSeq(6*(64*64*64+1)), editing.tailBlockSeq)
	should.Equal(slotIndexSeq(7*4161), editing.tailSlotIndexSeq)
	strategy := editing.strategy
	level0SlotIndex := getLevel(editing, 0)
	result := level0SlotIndex.searchSmall(strategy.NewBlobValueFilter(0, "hello262144"))
	should.Equal(biter.SetBits[0], result)
	level1SlotIndex := getLevel(editing, 1)
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "hello262144"))
	should.Equal(biter.SetBits[0], result)
	level2SlotIndex := getLevel(editing, 2)
	result = level2SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "hello262144"))
	should.Equal(biter.SetBits[0], result)
	should.Equal(level(3), editing.topLevel)
	level3SlotIndex := getLevel(editing, 3)
	result = level3SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "hello262143"))
	should.Equal(biter.SetBits[0], result)
	result = level3SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "hello262144"))
	should.Equal(biter.SetBits[1], result)
}

func Test_add_64x64x64x2_plus_1_blocks(t *testing.T) {
	blockLength = 2
	blockLengthInPowerOfTwo = 1
	should := require.New(t)
	editing := fakeEditingHead()
	for i := 0; i < 64*64*64*2+1; i++ {
		editing.addBlock(ctx, newBlock(0, []*Entry{
			blobEntry(Blob(fmt.Sprintf("hello%d", i))),
		}))
	}
	should.Equal(blockSeq(6*(64*64*64*2+1)), editing.tailBlockSeq)
	should.Equal(slotIndexSeq(7*8322), editing.tailSlotIndexSeq)
	strategy := editing.strategy
	level0SlotIndex := getLevel(editing, 0)
	result := level0SlotIndex.searchSmall(strategy.NewBlobValueFilter(0, "hello524288"))
	should.Equal(biter.SetBits[0], result)
	level1SlotIndex := getLevel(editing, 1)
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "hello524288"))
	should.Equal(biter.SetBits[0], result)
	level2SlotIndex := getLevel(editing, 2)
	result = level2SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "hello524288"))
	should.Equal(biter.SetBits[0], result)
	should.Equal(level(3), editing.topLevel)
	level3SlotIndex := getLevel(editing, 3)
	result = level3SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "hello262143"))
	should.Equal(biter.SetBits[0], result)
	result = level3SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "hello262144"))
	should.Equal(biter.SetBits[1], result)
	result = level3SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "hello524287"))
	should.Equal(biter.SetBits[1], result)
	result = level3SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "hello524288"))
	should.Equal(biter.SetBits[2], result)
}

func Test_add_64x64x64x64_plus_1_blocks(t *testing.T) {
	blockLength = 2
	blockLengthInPowerOfTwo = 1
	should := require.New(t)
	editing := fakeEditingHead()
	editing.tailOffset = Offset(64 * 64 * 64 * 64 * blockLength)
	editing.addBlock(ctx, newBlock(0, []*Entry{
		blobEntry("final block"),
	}))
	should.Equal(level(4), editing.topLevel)
	strategy := editing.strategy
	level0SlotIndex := getLevel(editing, 0)
	result := level0SlotIndex.searchSmall(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level1SlotIndex := getLevel(editing, 1)
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level2SlotIndex := getLevel(editing, 2)
	result = level2SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level3SlotIndex := getLevel(editing, 3)
	result = level3SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level4SlotIndex := getLevel(editing, 4)
	result = level4SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[1], result)
}

func Test_add_64x64x64x64x64_plus_1_blocks(t *testing.T) {
	blockLength = 2
	blockLengthInPowerOfTwo = 1
	should := require.New(t)
	editing := fakeEditingHead()
	editing.tailOffset = Offset(64 * 64 * 64 * 64 * 64 * blockLength)
	editing.addBlock(ctx, newBlock(0, []*Entry{
		blobEntry("final block"),
	}))
	should.Equal(level(5), editing.topLevel)
	strategy := editing.strategy
	level0SlotIndex := getLevel(editing, 0)
	result := level0SlotIndex.searchSmall(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level1SlotIndex := getLevel(editing, 1)
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level2SlotIndex := getLevel(editing, 2)
	result = level2SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level3SlotIndex := getLevel(editing, 3)
	result = level3SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level4SlotIndex := getLevel(editing, 4)
	result = level4SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level5SlotIndex := getLevel(editing, 5)
	result = level5SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[1], result)
}

func Test_add_64x64x64x64x64x64_plus_1_blocks(t *testing.T) {
	blockLength = 2
	blockLengthInPowerOfTwo = 1
	should := require.New(t)
	editing := fakeEditingHead()
	editing.tailOffset = Offset(64 * 64 * 64 * 64 * 64 * 64 * blockLength)
	editing.addBlock(ctx, newBlock(0, []*Entry{
		blobEntry("final block"),
	}))
	should.Equal(level(6), editing.topLevel)
	strategy := editing.strategy
	level0SlotIndex := getLevel(editing, 0)
	result := level0SlotIndex.searchSmall(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level1SlotIndex := getLevel(editing, 1)
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level2SlotIndex := getLevel(editing, 2)
	result = level2SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level3SlotIndex := getLevel(editing, 3)
	result = level3SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level4SlotIndex := getLevel(editing, 4)
	result = level4SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level5SlotIndex := getLevel(editing, 5)
	result = level5SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level6SlotIndex := getLevel(editing, 6)
	result = level6SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[1], result)
}

func Test_add_64x64x64x64x64x64x64_plus_1_blocks(t *testing.T) {
	blockLength = 2
	blockLengthInPowerOfTwo = 1
	should := require.New(t)
	editing := fakeEditingHead()
	editing.tailOffset = Offset(64 * 64 * 64 * 64 * 64 * 64 * 64 * blockLength)
	editing.addBlock(ctx, newBlock(0, []*Entry{
		blobEntry("final block"),
	}))
	editing.addBlock(ctx, newBlock(0, []*Entry{
		blobEntry("final final block"),
	}))
	should.Equal(level(7), editing.topLevel)
	strategy := editing.strategy
	level0SlotIndex := getLevel(editing, 0)
	result := level0SlotIndex.searchSmall(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	result = level0SlotIndex.searchSmall(strategy.NewBlobValueFilter(0, "final final block"))
	should.Equal(biter.SetBits[1], result)
	level1SlotIndex := getLevel(editing, 1)
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "final final block"))
	should.Equal(biter.SetBits[0], result)
	level2SlotIndex := getLevel(editing, 2)
	result = level2SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level3SlotIndex := getLevel(editing, 3)
	result = level3SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level4SlotIndex := getLevel(editing, 4)
	result = level4SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level5SlotIndex := getLevel(editing, 5)
	result = level5SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level6SlotIndex := getLevel(editing, 6)
	result = level6SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level7SlotIndex := getLevel(editing, 7)
	result = level7SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[1], result)
	result = level7SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final final block"))
	should.Equal(biter.SetBits[1], result)
}

func Test_add_64x64x64x64x64x64x64x64_plus_1_blocks(t *testing.T) {
	blockLength = 2
	blockLengthInPowerOfTwo = 1
	should := require.New(t)
	editing := fakeEditingHead()
	editing.tailOffset = Offset(64 * 64 * 64 * 64 * 64 * 64 * 64 * 64 * blockLength)
	editing.addBlock(ctx, newBlock(0, []*Entry{
		blobEntry("final block"),
	}))
	should.Equal(level(8), editing.topLevel)
	strategy := editing.strategy
	level0SlotIndex := getLevel(editing, 0)
	result := level0SlotIndex.searchSmall(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level1SlotIndex := getLevel(editing, 1)
	result = level1SlotIndex.searchMedium(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level2SlotIndex := getLevel(editing, 2)
	result = level2SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level3SlotIndex := getLevel(editing, 3)
	result = level3SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level4SlotIndex := getLevel(editing, 4)
	result = level4SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level5SlotIndex := getLevel(editing, 5)
	result = level5SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level6SlotIndex := getLevel(editing, 6)
	result = level6SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level7SlotIndex := getLevel(editing, 7)
	result = level7SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[0], result)
	level8SlotIndex := getLevel(editing, 8)
	result = level8SlotIndex.searchLarge(strategy.NewBlobValueFilter(0, "final block"))
	should.Equal(biter.SetBits[1], result)
}