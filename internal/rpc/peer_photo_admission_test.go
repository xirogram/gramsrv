package rpc

import (
	"context"
	"testing"

	"github.com/iamxvbaba/td/tg"
	appchannels "telesrv/internal/app/channels"
	"telesrv/internal/domain"
	"telesrv/internal/store/memory"
)

func TestPeerPhotoWithoutPhotoIDCannotResolvePrivateChannel(t *testing.T) {
	ctx := context.Background()
	channels := appchannels.NewService(memory.NewChannelStore())
	created, err := channels.CreateMegagroupFromCreateChat(ctx, 1001, domain.CreateChannelRequest{
		Title: "Private avatar", MemberUserIDs: []int64{1002}, Date: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = channels.SetPhoto(ctx, 1001, created.Channel.ID, &domain.Photo{ID: 9001, DCID: 2}, 11)
	if err != nil {
		t.Fatal(err)
	}
	files := &fakeFiles{getFileFound: true, getFileChunk: domain.FileChunk{Bytes: []byte("private avatar")}}
	r := &Router{deps: Deps{Channels: channels, Files: files}}
	_, err = r.onUploadGetFile(WithUserID(ctx, 1003), &tg.UploadGetFileRequest{
		Location: &tg.InputPeerPhotoFileLocation{
			Peer: &tg.InputPeerChannel{ChannelID: created.Channel.ID, AccessHash: created.Channel.AccessHash + 1},
		}, Limit: 1024,
	})
	if err == nil || files.getFileRequest.LocationKey != "" {
		t.Fatalf("non-member with invalid access hash downloaded private avatar: key=%q err=%v", files.getFileRequest.LocationKey, err)
	}
}

func TestProfilePhotoParsingPreservesDecodedFlags(t *testing.T) {
	req := &tg.InputChatUploadedPhoto{File: &tg.InputFile{ID: 123, Parts: 1, Name: "unflagged.jpg"}}
	_, mediaFlags, err := parseProfilePhotoUpload(req)
	if err != nil || mediaFlags != 0 || req.Flags != 0 {
		t.Fatalf("unflagged fields acquired protocol presence: flags=%v media=%d err=%v", req.Flags, mediaFlags, err)
	}
	req.SetFile(req.File)
	_, mediaFlags, err = parseProfilePhotoUpload(req)
	if err != nil || mediaFlags == 0 {
		t.Fatalf("flagged upload rejected: flags=%d err=%v", mediaFlags, err)
	}
}
