package lstore

import (
	"github.com/v2pro/plz/countlog"
)

func loadInitialVersion(ctx countlog.Context, config *Config) (*StoreVersion, error) {
	version := StoreVersion{config: *config}.edit()
	indexedSegment, err := openHeadSegment(ctx, config, config.indexingStrategy)
	ctx.TraceCall("callee!store.openHeadSegment", err)
	if err != nil {
		return nil, err
	}
	version.indexedSegment = indexedSegment
	err = loadTailAndRawSegments(config, version)
	ctx.TraceCall("callee!store.loadTailAndRawSegments", err)
	if err != nil {
		return nil, err
	}
	return version.seal(), nil
}

func loadTailAndRawSegments(config *Config, version *EditingStoreVersion) error {
	tailSegment, err := openTailSegment(config.TailSegmentPath(), config.TailSegmentMaxSize, 0)
	if err != nil {
		return err
	}
	var reversedRawSegments []*rawSegment
	startOffset := tailSegment.startOffset
	for startOffset != version.indexedSegment.tailOffset {
		rawSegmentPath := config.RawSegmentPath(startOffset)
		rawSegment, err := openRawSegment(rawSegmentPath)
		if err != nil {
			countlog.Error("event!lstore.failed to open raw segment",
				"err", err, "rawSegmentPath", rawSegmentPath)
			return err
		}
		reversedRawSegments = append(reversedRawSegments, rawSegment)
		startOffset = rawSegment.startOffset
	}
	rawSegments := make([]*rawSegment, len(reversedRawSegments))
	for i := 0; i < len(reversedRawSegments); i++ {
		rawSegments[i] = reversedRawSegments[len(reversedRawSegments)-i-1]
	}
	version.tailSegment = tailSegment
	version.rawSegments = rawSegments
	return nil
}