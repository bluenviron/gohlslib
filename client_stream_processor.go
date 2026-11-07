package gohlslib

import (
	"context"
)

type clientStreamProcessorParent interface {
	setTracks(errGroupCtx context.Context, tracks []*Track) ([]*clientTrack, bool)
	onProcessorError(errGroupCtx context.Context, err error)
	setTimeConv(ts clientTimeConv)
	waitTimeConv(errGroupCtx context.Context) (clientTimeConv, bool)
}
