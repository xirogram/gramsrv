package rpc

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tlprofile"
	"telesrv/internal/domain"
)

func TestFactCheckRegressionBatchKeepsMessagePositions(t *testing.T) {
	r, _, owner, channel, post := factCheckTestRouter(t)
	ctx := WithUserID(context.Background(), owner.ID)
	peer := &tg.InputPeerChannel{ChannelID: channel.ID, AccessHash: channel.AccessHash}
	if _, err := r.onMessagesEditFactCheck(ctx, &tg.MessagesEditFactCheckRequest{
		Peer: peer, MsgID: post.ID, Text: tg.TextWithEntities{Text: "belongs to post"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := r.onMessagesGetFactCheck(ctx, &tg.MessagesGetFactCheckRequest{Peer: peer, MsgID: []int{999999, post.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("requested [missing, post], got %d result(s): %+v; desktop assigns result[0] to the missing message", len(got), got)
	}
}

func TestFactCheckRegressionExactProfiles(t *testing.T) {
	for layer := 225; layer <= 231; layer++ {
		t.Run(strconv.Itoa(layer), func(t *testing.T) {
			r, _, owner, channel, post := factCheckTestRouter(t)
			ctx := WithUserID(context.Background(), owner.ID)
			peer := &tg.InputPeerChannel{ChannelID: channel.ID, AccessHash: channel.AccessHash}
			dispatchExactLayerRPCTest(t, r, ctx, tlprofile.Profile(layer), &tg.MessagesEditFactCheckRequest{Peer: peer, MsgID: post.ID, Text: tg.TextWithEntities{Text: "verdict"}})
			dispatchExactLayerRPCTest(t, r, ctx, tlprofile.Profile(layer), &tg.MessagesGetFactCheckRequest{Peer: peer, MsgID: []int{999999, post.ID, post.ID}})
			dispatchExactLayerRPCTest(t, r, ctx, tlprofile.Profile(layer), &tg.MessagesDeleteFactCheckRequest{Peer: peer, MsgID: post.ID})
		})
	}
}

func TestFactCheckRegressionRepeatedWriteDoesNotAllocatePts(t *testing.T) {
	r, store, owner, channel, post := factCheckTestRouter(t)
	ctx := WithUserID(context.Background(), owner.ID)
	req := &tg.MessagesEditFactCheckRequest{Peer: &tg.InputPeerChannel{ChannelID: channel.ID, AccessHash: channel.AccessHash}, MsgID: post.ID, Text: tg.TextWithEntities{Text: "same verdict"}}
	if _, err := r.onMessagesEditFactCheck(ctx, req); err != nil {
		t.Fatal(err)
	}
	before, err := store.ResolveChannel(ctx, owner.ID, channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.onMessagesEditFactCheck(ctx, req); err == nil || !strings.Contains(err.Error(), "MESSAGE_NOT_MODIFIED") {
		t.Fatalf("repeat write: %v", err)
	}
	after, err := store.ResolveChannel(ctx, owner.ID, channel.ID)
	if err != nil || after.Channel.Pts != before.Channel.Pts {
		t.Fatalf("repeat write advanced pts: %d -> %d, %v", before.Channel.Pts, after.Channel.Pts, err)
	}
}

func TestFactCheckRegressionEntityOnlyEditChangesHash(t *testing.T) {
	r, _, owner, channel, post := factCheckTestRouter(t)
	ctx := WithUserID(context.Background(), owner.ID)
	peer := &tg.InputPeerChannel{ChannelID: channel.ID, AccessHash: channel.AccessHash}
	save := func(url string) tg.FactCheck {
		_, err := r.onMessagesEditFactCheck(ctx, &tg.MessagesEditFactCheckRequest{
			Peer: peer, MsgID: post.ID, Text: tg.TextWithEntities{Text: "source", Entities: []tg.MessageEntityClass{&tg.MessageEntityTextURL{Offset: 0, Length: 6, URL: url}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.onMessagesGetFactCheck(ctx, &tg.MessagesGetFactCheckRequest{Peer: peer, MsgID: []int{post.ID}})
		if err != nil || len(got) != 1 {
			t.Fatalf("read: %+v %v", got, err)
		}
		return got[0]
	}
	a, b := save("https://example.com/a"), save("https://example.com/b")
	if a.Hash == b.Hash {
		t.Fatalf("link target changed but hash stayed %d: desktop ignores the new verdict", a.Hash)
	}
}

func TestFactCheckRegressionDeletedMessageHidesFactCheck(t *testing.T) {
	r, store, owner, channel, post := factCheckTestRouter(t)
	ctx := WithUserID(context.Background(), owner.ID)
	peer := &tg.InputPeerChannel{ChannelID: channel.ID, AccessHash: channel.AccessHash}
	if _, err := r.onMessagesEditFactCheck(ctx, &tg.MessagesEditFactCheckRequest{Peer: peer, MsgID: post.ID, Text: tg.TextWithEntities{Text: "removed verdict"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeleteChannelMessages(ctx, domain.DeleteChannelMessagesRequest{UserID: owner.ID, ChannelID: channel.ID, IDs: []int{post.ID}, Date: 1700100999}); err != nil {
		t.Fatal(err)
	}
	got, err := r.onMessagesGetFactCheck(ctx, &tg.MessagesGetFactCheckRequest{Peer: peer, MsgID: []int{post.ID}})
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range got {
		if check.Text.Text != "" {
			t.Fatalf("deleted message still exposes verdict: %+v", check)
		}
	}
}

func TestFactCheckRegressionWritePushesOtherSessions(t *testing.T) {
	r, store, owner, channel, post := factCheckTestRouter(t)
	subscriber := domain.User{ID: owner.ID + 1, AccessHash: 92, FirstName: "Subscriber"}
	r.deps.Users = &prefetchRecordingUsersService{mapUsersService: mapUsersService{users: map[int64]domain.User{owner.ID: owner, subscriber.ID: subscriber}}}
	if _, err := store.JoinChannel(context.Background(), channel.ID, subscriber.ID, 1700100000); err != nil {
		t.Fatal(err)
	}
	sessions := &captureSessions{onlineUserIDs: []int64{owner.ID, subscriber.ID}, channelMembers: map[int64][]int64{channel.ID: {owner.ID, subscriber.ID}}}
	r.deps.Sessions = sessions
	bg, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.RunChannelFanout(bg)
	deadline := time.Now().Add(2 * time.Second)
	for !r.channelFanout.started.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !r.channelFanout.started.Load() {
		t.Fatal("fanout did not start")
	}
	ctx := WithSessionID(WithRawAuthKeyID(WithUserID(context.Background(), owner.ID), [8]byte{1, 2, 3}), 123)
	before, err := store.ResolveChannel(ctx, owner.ID, channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	echo, err := r.onMessagesEditFactCheck(ctx, &tg.MessagesEditFactCheckRequest{
		Peer: &tg.InputPeerChannel{ChannelID: channel.ID, AccessHash: channel.AccessHash}, MsgID: post.ID, Text: tg.TextWithEntities{Text: "new verdict"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(echo.(*tg.Updates).Updates) != 1 {
		t.Fatal("current device did not receive one edit echo")
	}
	assertPush := func(wantText string) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			ids := sessions.pushedUserIDs()
			if fanoutHasID(ids, owner.ID) && fanoutHasID(ids, subscriber.ID) {
				break
			}
			time.Sleep(time.Millisecond)
		}
		ids := sessions.pushedUserIDs()
		if !fanoutHasID(ids, owner.ID) || !fanoutHasID(ids, subscriber.ID) {
			t.Fatalf("missing online delivery: %v", ids)
		}
		updates := sessions.lastUserPush().(*tg.Updates)
		msg := updates.Updates[0].(*tg.UpdateEditChannelMessage).Message.(*tg.Message)
		check, exists := msg.GetFactcheck()
		if exists != (wantText != "") || (exists && check.Text.Text != wantText) {
			t.Fatalf("pushed fact check = %+v, exists=%v", check, exists)
		}
		if sessions.snapshot().sessionID != 123 {
			t.Fatal("origin session exclusion lost")
		}
	}
	assertPush("new verdict")
	sessions.clearMessages()
	if _, err := r.onMessagesDeleteFactCheck(ctx, &tg.MessagesDeleteFactCheckRequest{Peer: &tg.InputPeerChannel{ChannelID: channel.ID, AccessHash: channel.AccessHash}, MsgID: post.ID}); err != nil {
		t.Fatal(err)
	}
	assertPush("")
	difference, err := store.ListChannelDifference(ctx, domain.ChannelDifferenceRequest{UserID: subscriber.ID, ChannelID: channel.ID, Pts: before.Channel.Pts, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(difference.Events) != 2 || difference.Events[0].Message.FactCheck == nil || difference.Events[1].Message.FactCheck != nil {
		t.Fatalf("offline edit/delete recovery = %+v", difference.Events)
	}
}

func TestFactCheckRegressionClearedHistoryAndBatchOrder(t *testing.T) {
	r, store, owner, channel, post := factCheckTestRouter(t)
	ctx := WithUserID(context.Background(), owner.ID)
	second, err := store.SendChannelMessage(ctx, domain.SendChannelMessageRequest{UserID: owner.ID, ChannelID: channel.ID, RandomID: 2, Message: "second", Date: post.Date + 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range []domain.ChannelMessage{post, second.Message} {
		if _, err := store.EditFactCheck(ctx, domain.EditFactCheckRequest{UserID: owner.ID, ChannelID: channel.ID, MessageID: msg.ID, Text: msg.Body}); err != nil {
			t.Fatal(err)
		}
	}
	peer := &tg.InputPeerChannel{ChannelID: channel.ID, AccessHash: channel.AccessHash}
	checks, err := r.onMessagesGetFactCheck(ctx, &tg.MessagesGetFactCheckRequest{Peer: peer, MsgID: []int{second.Message.ID, post.ID, second.Message.ID, 999999}})
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 4 || checks[0].Text.Text != "second" || checks[1].Text.Text != "post" || checks[2].Hash != checks[0].Hash || checks[3].Hash != 0 {
		t.Fatalf("ordered batch = %+v", checks)
	}
	if _, err := store.DeleteChannelHistory(ctx, domain.DeleteChannelHistoryRequest{UserID: owner.ID, ChannelID: channel.ID, MaxID: post.ID, Date: post.Date + 2}); err != nil {
		t.Fatal(err)
	}
	checks, err = r.onMessagesGetFactCheck(ctx, &tg.MessagesGetFactCheckRequest{Peer: peer, MsgID: []int{post.ID, second.Message.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 2 || checks[0].Hash != 0 || checks[1].Hash == 0 {
		t.Fatalf("cleared-history batch = %+v", checks)
	}
}
