// internal/ui/services_helpers_test.go
//
// Test-only helper methods on App that wire single service-method
// closures. The production surface takes a full XxxServiceFuncs
// bundle; most tests only need one closure, and these helpers
// preserve the original per-method SetXxx call style without
// polluting the production API.
//
// For services where tests routinely chain multiple SetXxx calls
// (notably ChannelService — many tests wire ReadCache + SyncedAt +
// Fetch + MarkRead together), the helpers remember the closures each
// App was last wired with, so each helper call preserves previously-set
// funcs.
//
// File name ends in _test.go so these are invisible outside the test
// binary.
package ui

import (
	"sync"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
)

func (a *App) setThreadFetcherForTest(fn core.ThreadFetchFunc) {
	a.SetThreadService(core.NewThreadService(core.ThreadServiceFuncs{Fetch: fn}))
}

func (a *App) setThreadsListFetcherForTest(fn core.ThreadsListFetchFunc) {
	a.SetThreadService(core.NewThreadService(core.ThreadServiceFuncs{ListFetch: fn}))
}

func (a *App) setPermalinkFetcherForTest(fn core.PermalinkFetchFunc) {
	a.SetMessageService(core.NewMessageService(core.MessageServiceFuncs{Permalink: fn}))
}

var (
	channelFuncsMu sync.Mutex
	channelFuncs   = map[*App]core.ChannelServiceFuncs{}
)

// setChannelFuncsForTest installs a ChannelService built from fns and
// remembers fns, so the per-method helpers below can add one closure at
// a time without dropping the others.
func setChannelFuncsForTest(a *App, fns core.ChannelServiceFuncs) {
	channelFuncsMu.Lock()
	channelFuncs[a] = fns
	channelFuncsMu.Unlock()
	a.SetChannelService(core.NewChannelService(fns))
}

// channelFuncsForTest returns the closures a's ChannelService was last
// built from via setChannelFuncsForTest.
func channelFuncsForTest(a *App) core.ChannelServiceFuncs {
	channelFuncsMu.Lock()
	defer channelFuncsMu.Unlock()
	return channelFuncs[a]
}

func (a *App) setChannelFetcherForTest(fn core.ChannelFetchFunc) {
	fns := channelFuncsForTest(a)
	fns.Fetch = fn
	setChannelFuncsForTest(a, fns)
}

func (a *App) setChannelReadMarkerForTest(fn func(channelID ids.ChannelID, ts ids.MessageTS) core.Msg) {
	fns := channelFuncsForTest(a)
	fns.MarkRead = fn
	setChannelFuncsForTest(a, fns)
}

func (a *App) setChannelCacheReaderForTest(fn core.ChannelCacheReadFunc) {
	fns := channelFuncsForTest(a)
	fns.ReadCache = fn
	setChannelFuncsForTest(a, fns)
}

func (a *App) setChannelSyncedAtReaderForTest(fn func(channelID ids.ChannelID) int64) {
	fns := channelFuncsForTest(a)
	fns.SyncedAt = fn
	setChannelFuncsForTest(a, fns)
}

func (a *App) setOlderMessagesFetcherForTest(fn core.OlderMessagesFetchFunc) {
	fns := channelFuncsForTest(a)
	fns.FetchOlder = fn
	setChannelFuncsForTest(a, fns)
}

func setChannelFetchAroundForTest(a *App, fn func(channelID ids.ChannelID, ts ids.MessageTS) core.Msg) {
	fns := channelFuncsForTest(a)
	fns.FetchAround = fn
	setChannelFuncsForTest(a, fns)
}

func (a *App) setChannelLookupFuncForTest(fn core.ChannelLookupFunc) {
	fns := channelFuncsForTest(a)
	fns.Lookup = fn
	setChannelFuncsForTest(a, fns)
}

func (a *App) setChannelVisitRecorderForTest(fn core.ChannelVisitRecorder) {
	fns := channelFuncsForTest(a)
	fns.RecordVisit = fn
	setChannelFuncsForTest(a, fns)
}

func (a *App) setChannelMembershipFetcherForTest(fn func(channelID ids.ChannelID)) {
	fns := channelFuncsForTest(a)
	fns.MembershipFetch = fn
	setChannelFuncsForTest(a, fns)
}
