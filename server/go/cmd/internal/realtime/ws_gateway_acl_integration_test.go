package realtime

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"arc/cmd/internal/auth/session"
	v1 "arc/shared/contracts/realtime/v1"
)

func TestWSGateway_Join_PublicConversation_Allowed(t *testing.T) {
	t.Setenv("ARC_WS_DEV_INSECURE", "false")
	t.Setenv("ARC_WS_REQUIRE_AUTH", "true")
	t.Setenv("ARC_WS_REQUIRE_MEMBERSHIP", "true")
	t.Setenv("ARC_WS_ORIGIN_REQUIRED", "false")

	now := time.Now().UTC()
	row := session.Row{
		ID:        "sess-acl-public-1",
		UserID:    "user-acl-public-1",
		CreatedAt: now,
		ExpiresAt: now.Add(1 * time.Hour),
		Platform:  session.PlatformWeb,
	}

	authSvc, tokens := newWSAuthService(t, row, 15*time.Minute)
	accessToken, _, err := tokens.Issue(row.UserID, row.ID, now)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	members := newWSACLMembershipStore()
	members.putConversation(ConversationInfo{
		ID:         "conv-public-room-1",
		Kind:       "room",
		Visibility: conversationVisibilityPublic,
	})

	gw := newWSACLGateway(t, authSvc, members)
	ts := startWSTestServer(t, gw)
	defer ts.Close()

	conn, resp, err := dialWS(t, ts.URL, wsDialInput{Bearer: accessToken})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("authorized dial failed: %v", err)
	}
	defer func() { _ = conn.Close(1000, "bye") }()

	writeEnvelopeWS(t, conn, v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeConversationJoin,
		ID:   "join-public-1",
		TS:   time.Now().UTC(),
		Payload: mustJSONRaw(t, v1.ConversationJoinPayload{
			ConversationID: "conv-public-room-1",
			Kind:           "room",
		}),
	})

	joinEnv := readUntilType(t, conn, v1.TypeConversationJoin, 4)
	var joinPayload v1.ConversationJoinPayload
	if err := json.Unmarshal(joinEnv.Payload, &joinPayload); err != nil {
		t.Fatalf("decode join payload: %v", err)
	}
	if joinPayload.ConversationID != "conv-public-room-1" {
		t.Fatalf("expected conversation_id=conv-public-room-1, got %q", joinPayload.ConversationID)
	}
	if joinPayload.Kind != "room" {
		t.Fatalf("expected kind=room, got %q", joinPayload.Kind)
	}
}

func TestWSGateway_Join_PrivateConversation_DeniedForNonMember(t *testing.T) {
	t.Setenv("ARC_WS_DEV_INSECURE", "false")
	t.Setenv("ARC_WS_REQUIRE_AUTH", "true")
	t.Setenv("ARC_WS_REQUIRE_MEMBERSHIP", "true")
	t.Setenv("ARC_WS_ORIGIN_REQUIRED", "false")

	now := time.Now().UTC()
	row := session.Row{
		ID:        "sess-acl-private-1",
		UserID:    "user-acl-private-1",
		CreatedAt: now,
		ExpiresAt: now.Add(1 * time.Hour),
		Platform:  session.PlatformWeb,
	}

	authSvc, tokens := newWSAuthService(t, row, 15*time.Minute)
	accessToken, _, err := tokens.Issue(row.UserID, row.ID, now)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	members := newWSACLMembershipStore()
	members.putConversation(ConversationInfo{
		ID:         "conv-private-1",
		Kind:       "group",
		Visibility: conversationVisibilityPrivate,
	})

	gw := newWSACLGateway(t, authSvc, members)
	ts := startWSTestServer(t, gw)
	defer ts.Close()

	conn, resp, err := dialWS(t, ts.URL, wsDialInput{Bearer: accessToken})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("authorized dial failed: %v", err)
	}
	defer func() { _ = conn.Close(1000, "bye") }()

	writeEnvelopeWS(t, conn, v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeConversationJoin,
		ID:   "join-private-1",
		TS:   time.Now().UTC(),
		Payload: mustJSONRaw(t, v1.ConversationJoinPayload{
			ConversationID: "conv-private-1",
			Kind:           "group",
		}),
	})

	errEnv := readUntilType(t, conn, v1.TypeError, 4)
	var p v1.ErrorPayload
	if err := json.Unmarshal(errEnv.Payload, &p); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if p.Code != "join_failed" {
		t.Fatalf("expected code=join_failed, got %q", p.Code)
	}
	if !strings.Contains(strings.ToLower(p.Message), "not a member") {
		t.Fatalf("expected membership denial message, got %q", p.Message)
	}
}

func TestWSGateway_Join_UnknownVisibility_FailsClosed(t *testing.T) {
	t.Setenv("ARC_WS_DEV_INSECURE", "false")
	t.Setenv("ARC_WS_REQUIRE_AUTH", "true")
	t.Setenv("ARC_WS_REQUIRE_MEMBERSHIP", "true")
	t.Setenv("ARC_WS_ORIGIN_REQUIRED", "false")

	now := time.Now().UTC()
	row := session.Row{
		ID:        "sess-acl-unknown-vis-1",
		UserID:    "user-acl-unknown-vis-1",
		CreatedAt: now,
		ExpiresAt: now.Add(1 * time.Hour),
		Platform:  session.PlatformWeb,
	}

	authSvc, tokens := newWSAuthService(t, row, 15*time.Minute)
	accessToken, _, err := tokens.Issue(row.UserID, row.ID, now)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	members := newWSACLMembershipStore()
	members.putConversation(ConversationInfo{
		ID:         "conv-unknown-vis-1",
		Kind:       "group",
		Visibility: "legacy_value",
	})

	gw := newWSACLGateway(t, authSvc, members)
	ts := startWSTestServer(t, gw)
	defer ts.Close()

	conn, resp, err := dialWS(t, ts.URL, wsDialInput{Bearer: accessToken})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("authorized dial failed: %v", err)
	}
	defer func() { _ = conn.Close(1000, "bye") }()

	writeEnvelopeWS(t, conn, v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeConversationJoin,
		ID:   "join-unknown-vis-1",
		TS:   time.Now().UTC(),
		Payload: mustJSONRaw(t, v1.ConversationJoinPayload{
			ConversationID: "conv-unknown-vis-1",
			Kind:           "group",
		}),
	})

	errEnv := readUntilType(t, conn, v1.TypeError, 4)
	var p v1.ErrorPayload
	if err := json.Unmarshal(errEnv.Payload, &p); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if p.Code != "join_failed" {
		t.Fatalf("expected code=join_failed, got %q", p.Code)
	}
	if !strings.Contains(strings.ToLower(p.Message), "not a member") {
		t.Fatalf("expected membership denial message, got %q", p.Message)
	}
}

func TestWSGateway_SendAndHistory_DeniedForNonMember(t *testing.T) {
	t.Setenv("ARC_WS_DEV_INSECURE", "false")
	t.Setenv("ARC_WS_REQUIRE_AUTH", "true")
	t.Setenv("ARC_WS_REQUIRE_MEMBERSHIP", "true")
	t.Setenv("ARC_WS_ORIGIN_REQUIRED", "false")

	now := time.Now().UTC()
	row := session.Row{
		ID:        "sess-acl-send-1",
		UserID:    "user-acl-send-1",
		CreatedAt: now,
		ExpiresAt: now.Add(1 * time.Hour),
		Platform:  session.PlatformWeb,
	}

	authSvc, tokens := newWSAuthService(t, row, 15*time.Minute)
	accessToken, _, err := tokens.Issue(row.UserID, row.ID, now)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	convID := "conv-public-2"
	members := newWSACLMembershipStore()
	members.putConversation(ConversationInfo{
		ID:         convID,
		Kind:       "room",
		Visibility: conversationVisibilityPublic,
	})

	gw := newWSACLGateway(t, authSvc, members)
	ts := startWSTestServer(t, gw)
	defer ts.Close()

	conn, resp, err := dialWS(t, ts.URL, wsDialInput{Bearer: accessToken})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("authorized dial failed: %v", err)
	}
	defer func() { _ = conn.Close(1000, "bye") }()

	writeEnvelopeWS(t, conn, v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeConversationJoin,
		ID:   "join-public-2",
		TS:   time.Now().UTC(),
		Payload: mustJSONRaw(t, v1.ConversationJoinPayload{
			ConversationID: convID,
			Kind:           "room",
		}),
	})
	_ = readUntilType(t, conn, v1.TypeConversationJoin, 4)

	writeEnvelopeWS(t, conn, v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeMessageSend,
		ID:   "send-public-2",
		TS:   time.Now().UTC(),
		Payload: mustJSONRaw(t, v1.MessageSendPayload{
			ConversationID: convID,
			ClientMsgID:    "client-msg-denied-1",
			Text:           "hello denied",
		}),
	})

	sendErr := readUntilType(t, conn, v1.TypeError, 4)
	var sendErrPayload v1.ErrorPayload
	if err := json.Unmarshal(sendErr.Payload, &sendErrPayload); err != nil {
		t.Fatalf("decode send error payload: %v", err)
	}
	if sendErrPayload.Code != "send_failed" {
		t.Fatalf("expected code=send_failed, got %q", sendErrPayload.Code)
	}
	if !strings.Contains(strings.ToLower(sendErrPayload.Message), "not a member") {
		t.Fatalf("expected membership denial message, got %q", sendErrPayload.Message)
	}

	writeEnvelopeWS(t, conn, v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeConversationHistoryFetch,
		ID:   "history-public-2",
		TS:   time.Now().UTC(),
		Payload: mustJSONRaw(t, v1.ConversationHistoryFetchPayload{
			ConversationID: convID,
			Limit:          20,
		}),
	})

	historyErr := readUntilType(t, conn, v1.TypeError, 4)
	var historyErrPayload v1.ErrorPayload
	if err := json.Unmarshal(historyErr.Payload, &historyErrPayload); err != nil {
		t.Fatalf("decode history error payload: %v", err)
	}
	if historyErrPayload.Code != "history_failed" {
		t.Fatalf("expected code=history_failed, got %q", historyErrPayload.Code)
	}
	if !strings.Contains(strings.ToLower(historyErrPayload.Message), "not a member") {
		t.Fatalf("expected membership denial message, got %q", historyErrPayload.Message)
	}
}

func TestWSGateway_PrivateConversation_MemberCanJoinSendAndFetchHistory(t *testing.T) {
	t.Setenv("ARC_WS_DEV_INSECURE", "false")
	t.Setenv("ARC_WS_REQUIRE_AUTH", "true")
	t.Setenv("ARC_WS_REQUIRE_MEMBERSHIP", "true")
	t.Setenv("ARC_WS_ORIGIN_REQUIRED", "false")

	now := time.Now().UTC()
	row := session.Row{
		ID:        "sess-acl-private-member-1",
		UserID:    "user-acl-private-member-1",
		CreatedAt: now,
		ExpiresAt: now.Add(1 * time.Hour),
		Platform:  session.PlatformWeb,
	}

	authSvc, tokens := newWSAuthService(t, row, 15*time.Minute)
	accessToken, _, err := tokens.Issue(row.UserID, row.ID, now)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	convID := "conv-private-member-1"
	members := newWSACLMembershipStore()
	members.putConversation(ConversationInfo{
		ID:         convID,
		Kind:       "group",
		Visibility: conversationVisibilityPrivate,
	})
	members.putMember(convID, row.UserID)

	gw := newWSACLGateway(t, authSvc, members)
	ts := startWSTestServer(t, gw)
	defer ts.Close()

	conn, resp, err := dialWS(t, ts.URL, wsDialInput{Bearer: accessToken})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("authorized dial failed: %v", err)
	}
	defer func() { _ = conn.Close(1000, "bye") }()

	writeEnvelopeWS(t, conn, v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeConversationJoin,
		ID:   "join-private-member-1",
		TS:   time.Now().UTC(),
		Payload: mustJSONRaw(t, v1.ConversationJoinPayload{
			ConversationID: convID,
			Kind:           "group",
		}),
	})

	joinEnv := readUntilType(t, conn, v1.TypeConversationJoin, 4)
	var joinPayload v1.ConversationJoinPayload
	if err := json.Unmarshal(joinEnv.Payload, &joinPayload); err != nil {
		t.Fatalf("decode join payload: %v", err)
	}
	if joinPayload.ConversationID != convID {
		t.Fatalf("expected conversation_id=%q, got %q", convID, joinPayload.ConversationID)
	}
	if joinPayload.Kind != "group" {
		t.Fatalf("expected kind=group, got %q", joinPayload.Kind)
	}

	writeEnvelopeWS(t, conn, v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeMessageSend,
		ID:   "send-private-member-1",
		TS:   time.Now().UTC(),
		Payload: mustJSONRaw(t, v1.MessageSendPayload{
			ConversationID: convID,
			ClientMsgID:    "client-msg-private-member-1",
			Text:           "hello private member",
		}),
	})

	ackEnv := readUntilType(t, conn, v1.TypeMessageAck, 6)
	var ackPayload v1.MessageAckPayload
	if err := json.Unmarshal(ackEnv.Payload, &ackPayload); err != nil {
		t.Fatalf("decode message ack payload: %v", err)
	}
	if ackPayload.ConversationID != convID {
		t.Fatalf("expected ack conversation_id=%q, got %q", convID, ackPayload.ConversationID)
	}
	if ackPayload.ClientMsgID != "client-msg-private-member-1" {
		t.Fatalf("expected ack client_msg_id=%q, got %q", "client-msg-private-member-1", ackPayload.ClientMsgID)
	}
	if ackPayload.Seq <= 0 {
		t.Fatalf("expected positive sequence, got %d", ackPayload.Seq)
	}

	writeEnvelopeWS(t, conn, v1.Envelope{
		V:    v1.Version,
		Type: v1.TypeConversationHistoryFetch,
		ID:   "history-private-member-1",
		TS:   time.Now().UTC(),
		Payload: mustJSONRaw(t, v1.ConversationHistoryFetchPayload{
			ConversationID: convID,
			Limit:          20,
		}),
	})

	chunkEnv := readUntilType(t, conn, v1.TypeConversationHistoryChunk, 6)
	var chunkPayload v1.ConversationHistoryChunkPayload
	if err := json.Unmarshal(chunkEnv.Payload, &chunkPayload); err != nil {
		t.Fatalf("decode history chunk payload: %v", err)
	}
	if chunkPayload.ConversationID != convID {
		t.Fatalf("expected history conversation_id=%q, got %q", convID, chunkPayload.ConversationID)
	}
	if len(chunkPayload.Messages) == 0 {
		t.Fatalf("expected at least one history message")
	}
}

func newWSACLGateway(t *testing.T, authSvc *session.Service, members MembershipStore) *WSGateway {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewWSGateway(log, NewHub(log), NewInMemoryStore(), authSvc, members)
}

type wsACLMembershipStore struct {
	mu            sync.RWMutex
	conversations map[string]ConversationInfo
	members       map[string]map[string]struct{}
}

func newWSACLMembershipStore() *wsACLMembershipStore {
	return &wsACLMembershipStore{
		conversations: make(map[string]ConversationInfo),
		members:       make(map[string]map[string]struct{}),
	}
}

func (s *wsACLMembershipStore) putConversation(info ConversationInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conversations[info.ID] = info
}

func (s *wsACLMembershipStore) putMember(conversationID, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.members[conversationID]
	if m == nil {
		m = make(map[string]struct{})
		s.members[conversationID] = m
	}
	m[userID] = struct{}{}
}

func (s *wsACLMembershipStore) GetConversation(_ context.Context, conversationID string) (ConversationInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	info, ok := s.conversations[conversationID]
	if !ok {
		return ConversationInfo{}, ErrConversationNotFound
	}
	return info, nil
}

func (s *wsACLMembershipStore) IsMember(_ context.Context, userID, conversationID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.members[conversationID]
	if m == nil {
		return false, nil
	}
	_, ok := m[userID]
	return ok, nil
}

func (s *wsACLMembershipStore) EnsureMember(ctx context.Context, userID, conversationID string) error {
	if _, err := s.GetConversation(ctx, conversationID); err != nil {
		return err
	}
	ok, err := s.IsMember(ctx, userID, conversationID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrMembershipRequired
	}
	return nil
}

func (s *wsACLMembershipStore) AddMember(ctx context.Context, userID, conversationID string) error {
	info, err := s.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}
	if info.Visibility != conversationVisibilityPrivate {
		return ErrConversationNotPrivate
	}
	s.putMember(conversationID, userID)
	return nil
}

var _ MembershipStore = (*wsACLMembershipStore)(nil)
