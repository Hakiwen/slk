// internal/ui/callbacks.go
//
// Callback function types used to inject collaborators into App from
// cmd/slk/main.go. Each Set* method on App takes one of these types
// and stores it; the App invokes them in response to user actions.
//
// Phase 1 of the SOLID refactor of internal/ui/app.go: this file
// collects every callback type that previously lived in app.go. The
// callbacks themselves are still flat function pointers — Phase 3
// will group cohesive subsets into service interfaces (ChannelService,
// MessageService, ThreadService, ReactionService, WorkspaceService).
//
// No semantic change in this commit: same package, same declarations.
package ui

import (
	tea "charm.land/bubbletea/v2"
	"golang.design/x/clipboard"

	"github.com/gammons/slk/internal/ui/compose"
)

// SwitchWorkspaceFunc is called to switch the active workspace.
type SwitchWorkspaceFunc func(teamID string) tea.Msg

// UploadFunc performs an upload of one or more files to a channel
// (with optional thread). It returns a tea.Cmd whose terminal
// message is UploadResultMsg; intermediate UploadProgressMsg events
// are dispatched out-of-band via program.Send.
type UploadFunc func(channelID, threadTS, caption string, attachments []compose.PendingAttachment) tea.Cmd

// TypingSendFunc is called to broadcast a typing indicator.
type TypingSendFunc func(channelID string)

// clipboardReader abstracts clipboard.Read so tests can inject fake
// clipboard contents. Production code uses the real clipboard.Read.
type clipboardReader func(format clipboard.Format) []byte

// defaultClipboardReader is the real clipboard read function. It's
// overridable per-App via SetClipboardReader for tests.
var defaultClipboardReader clipboardReader = clipboard.Read

// clipboardWriter creates a Bubble Tea command that writes text through the
// terminal. Production uses tea.SetClipboard, which emits OSC 52 without
// invoking the native clipboard library or requiring CGO.
type clipboardWriter func(text string) tea.Cmd

// defaultClipboardWriter is overridable per-App for tests.
var defaultClipboardWriter clipboardWriter = tea.SetClipboard

// StatusReportFunc mirrors slk's unread state onto an external surface. It is
// called by notifyReadStateChanged on every read-state change with the
// active-workspace unread count, the other-workspace unread count, the active
// workspace name, and the window-title string. See notifications.status_command.
type StatusReportFunc func(unread, otherUnread int, workspace, title string)
