// Package mcp provides test utilities for MCP protocol testing.
// file: test/mcp/setup_test.go
package mcp

import (
	"testing"
	"time"

	"github.com/cowgnition/cowgnition/test/mocks"
)

// SetupMockRTMResponses configures the mock RTM server with required responses.
func SetupMockRTMResponses(rtmMock *mocks.RTMServer) {
	// Authentication-related responses
	rtmMock.AddResponse("rtm.auth.getFrob", `<rsp stat="ok"><frob>test_frob_12345</frob></rsp>`)
	rtmMock.AddResponse("rtm.auth.getToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)
	rtmMock.AddResponse("rtm.auth.checkToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)

	// Timeline-related responses
	rtmMock.AddResponse("rtm.timelines.create", `<rsp stat="ok"><timeline>timeline_12345</timeline></rsp>`)

	// Task and list related responses
	rtmMock.AddResponse("rtm.lists.getList", `<rsp stat="ok"><lists><list id="1" name="Inbox" deleted="0" locked="1" archived="0" position="-1" smart="0" /></lists></rsp>`)
	rtmMock.AddResponse("rtm.tasks.getList", `<rsp stat="ok"><tasks><list id="1"><taskseries id="1" created="2025-03-15T12:00:00Z" modified="2025-03-15T12:00:00Z" name="Test Task" source="api"><tags /><participants /><notes /><task id="1" due="" has_due_time="0" added="2025-03-15T12:00:00Z" completed="" deleted="" priority="N" postponed="0" estimate="" /></taskseries></list></tasks></rsp>`)

	// Tool-related responses
	rtmMock.AddResponse("rtm.tasks.add", `<rsp stat="ok"><transaction id="1" undoable="1" /><list id="1"><taskseries id="1" created="2025-03-15T12:00:00Z" modified="2025-03-15T12:00:00Z" name="New Task" source="api"><tags /><participants /><notes /><task id="1" due="" has_due_time="0" added="2025-03-15T12:00:00Z" completed="" deleted="" priority="N" postponed="0" estimate="" /></taskseries></list></rsp>`)

	// Error responses for testing
	rtmMock.AddResponse("rtm.error.test", `<rsp stat="fail"><err code="101" msg="Test error message" /></rsp>`)
}

// Helper function to create a mock server with default responses
func CreateMockServer(t *testing.T) *mocks.RTMServer {
	t.Helper()

	// Create a mock RTM server
	rtmMock := mocks.NewRTMServer(t)

	// Setup standard responses
	SetupMockRTMResponses(rtmMock)

	return rtmMock
}

// CreateTimeString returns a consistent time string for test data
func CreateTimeString() string {
	return time.Now().Format(time.RFC3339)
}
