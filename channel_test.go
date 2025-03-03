package stream_chat // nolint: golint

import (
	"log"
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_CreateChannel(t *testing.T) {
	c := initClient(t)

	userID := randomUser(t, c).ID

	t.Run("get existing channel", func(t *testing.T) {
		membersID := randomUsersID(t, c, 3)
		ch := initChannel(t, c, membersID...)
		got, err := c.CreateChannel(ch.Type, ch.ID, userID, nil)
		require.NoError(t, err, "create channel", ch)

		assert.Equal(t, c, got.client, "client link")
		assert.Equal(t, ch.Type, got.Type, "channel type")
		assert.Equal(t, ch.ID, got.ID, "channel id")
		assert.Equal(t, ch.MemberCount, got.MemberCount, "member count")
		assert.Len(t, got.Members, ch.MemberCount, "members length")
	})

	tests := []struct {
		name        string
		channelType string
		id          string
		userID      string
		data        map[string]interface{}
		wantErr     bool
	}{
		{"create channel with ID", "messaging", randomString(12), userID, nil, false},
		{"create channel without ID and members", "messaging", "", userID, nil, true},
		{
			"create channel without ID but with members", "messaging", "", userID,
			map[string]interface{}{"members": randomUsersID(t, c, 2)},
			false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.CreateChannel(tt.channelType, tt.id, tt.userID, tt.data)
			if tt.wantErr {
				require.Error(t, err, "create channel", tt)
				return
			}

			require.NoError(t, err, "create channel", tt)

			assert.Equal(t, tt.channelType, got.Type, "channel type")
			assert.NotEmpty(t, got.ID)
			if tt.id != "" {
				assert.Equal(t, tt.id, got.ID, "channel id")
			}
			assert.Equal(t, tt.userID, got.CreatedBy.ID, "channel created by")
		})
	}
}

func TestChannel_AddMembers(t *testing.T) {
	c := initClient(t)

	chanID := randomString(12)
	ch, err := c.CreateChannel("messaging", chanID, randomUser(t, c).ID, nil)
	require.NoError(t, err, "create channel")
	defer func() {
		_ = ch.Delete()
	}()

	assert.Empty(t, ch.Members, "members are empty")

	user := randomUser(t, c)
	options := map[string]interface{}{
		"hide_history": true,
	}
	err = ch.AddMembers(
		[]string{user.ID},
		&Message{Text: "some members", User: &User{ID: user.ID}},
		options,
	)
	require.NoError(t, err, "add members")

	// refresh channel state
	require.NoError(t, ch.refresh(), "refresh channel")
	assert.Equal(t, user.ID, ch.Members[0].User.ID, "members contain user id")
}

func TestChannel_ImportChannelMessages(t *testing.T) {
	c := initClient(t)

	owner := randomUser(t, c)
	chanID := randomString(12)

	ch, err := c.CreateChannel("messaging", chanID, owner.ID, nil)
	require.NoError(t, err, "create channel")
	defer func() {
		_ = ch.Delete()
	}()

	assert.Empty(t, ch.Members, "members are empty")

	user := randomUser(t, c)

	t0 := time.Unix(0, 0).UTC()
	t1 := time.Unix(1, 0).UTC()
	resp, err := ch.ImportMessages(
		&Message{
			Text:      "hi 1",
			User:      &User{ID: user.ID},
			CreatedAt: &t1,
		}, &Message{
			Text:      "hi 0",
			User:      &User{ID: user.ID},
			CreatedAt: &t0,
		})
	require.NoError(t, err, "Send bulk messages")
	require.Len(t, resp.Messages, 2)
	require.Equal(t, &t0, resp.Messages[0].CreatedAt)
	require.Equal(t, &t1, resp.Messages[1].CreatedAt)

	// get the channel and validate last_message_at
	ch, err = c.CreateChannel("messaging", chanID, owner.ID, nil)
	require.NoError(t, err, "create channel")
	require.Equal(t, t1, ch.LastMessageAt)
}

func TestChannel_QueryMembers(t *testing.T) {
	c := initClient(t)

	chanID := randomString(12)

	ch, err := c.CreateChannel("messaging", chanID, randomUser(t, c).ID, nil)
	require.NoError(t, err, "create channel")
	defer func() {
		_ = ch.Delete()
	}()

	assert.Empty(t, ch.Members, "members are empty")

	prefix := randomString(12)
	names := []string{"paul", "george", "john", "jessica", "john2"}

	for _, name := range names {
		id := prefix + name
		_, err := c.UpsertUser(&User{ID: id, Name: id})
		require.NoError(t, err)
		require.NoError(t, ch.AddMembers([]string{id}, nil, nil))
	}

	members, err := ch.QueryMembers(&QueryOption{
		Filter: map[string]interface{}{
			"name": map[string]interface{}{"$autocomplete": prefix + "j"},
		},
		Offset: 1,
		Limit:  10,
	}, &SortOption{Field: "created_at", Direction: 1})

	require.NoError(t, err)
	require.Len(t, members, 2)
	require.Equal(t, prefix+"jessica", members[0].User.ID)
	require.Equal(t, prefix+"john2", members[1].User.ID)
}

// See https://getstream.io/chat/docs/channel_members/ for more details.
func ExampleChannel_AddModerators() {
	channel := &Channel{}
	newModerators := []string{"bob", "sue"}

	_ = channel.AddModerators("thierry", "josh")
	_ = channel.AddModerators(newModerators...)
	_ = channel.DemoteModerators(newModerators...)
}

func TestChannel_InviteMembers(t *testing.T) {
	c := initClient(t)

	chanID := randomString(12)

	ch, err := c.CreateChannel("messaging", chanID, randomUser(t, c).ID, nil)
	require.NoError(t, err, "create channel")
	defer func() {
		_ = ch.Delete()
	}()

	assert.Empty(t, ch.Members, "members are empty")

	user := randomUser(t, c)

	err = ch.InviteMembers(user.ID)
	require.NoError(t, err, "invite members")

	// refresh channel state
	require.NoError(t, ch.refresh(), "refresh channel")

	assert.Equal(t, user.ID, ch.Members[0].User.ID, "members contain user id")
	assert.Equal(t, true, ch.Members[0].Invited, "member is invited")
	assert.Nil(t, ch.Members[0].InviteAcceptedAt, "invite is not accepted")
	assert.Nil(t, ch.Members[0].InviteRejectedAt, "invite is not rejected")
}

func TestChannel_Moderation(t *testing.T) {
	c := initClient(t)

	// init random channel
	chanID := randomString(12)
	ch, err := c.CreateChannel("messaging", chanID, randomUser(t, c).ID, nil)
	require.NoError(t, err, "create channel")
	defer func() {
		_ = ch.Delete()
	}()

	assert.Empty(t, ch.Members, "members are empty")

	user := randomUser(t, c)

	err = ch.AddModeratorsWithMessage(
		[]string{user.ID},
		&Message{Text: "accepted", User: &User{ID: user.ID}},
	)

	require.NoError(t, err, "add moderators")

	// refresh channel state
	require.NoError(t, ch.refresh(), "refresh channel")

	assert.Equal(t, user.ID, ch.Members[0].User.ID, "user exists")
	assert.Equal(t, "moderator", ch.Members[0].Role, "user role is moderator")

	err = ch.DemoteModerators(user.ID)
	require.NoError(t, err, "demote moderators")

	// refresh channel state
	require.NoError(t, ch.refresh(), "refresh channel")

	assert.Equal(t, user.ID, ch.Members[0].User.ID, "user exists")
	assert.Equal(t, "member", ch.Members[0].Role, "user role is member")
}

func TestChannel_BanUser(t *testing.T) {
	c := initClient(t)
	ch := initChannel(t, c)
	defer func() {
		_ = ch.Delete()
	}()

	user := randomUser(t, c)
	target := randomUser(t, c)

	err := ch.BanUser(target.ID, user.ID, nil)
	require.NoError(t, err, "ban user")

	err = ch.BanUser(target.ID, user.ID, map[string]interface{}{
		"timeout": 3600,
		"reason":  "offensive language is not allowed here",
	})
	require.NoError(t, err, "ban user")

	err = ch.UnBanUser(target.ID, nil)
	require.NoError(t, err, "unban user")
}

func TestChannel_Delete(t *testing.T) {
	c := initClient(t)
	ch := initChannel(t, c)

	require.NoError(t, ch.Delete(), "delete channel")
}

func TestChannel_GetReplies(t *testing.T) {
	c := initClient(t)
	ch := initChannel(t, c)
	defer func() {
		_ = ch.Delete()
	}()

	msg := &Message{Text: "test message"}

	msg, err := ch.SendMessage(msg, randomUser(t, c).ID, MessageSkipPush)
	require.NoError(t, err, "send message")

	reply := &Message{Text: "test reply", ParentID: msg.ID, Type: MessageTypeReply}
	_, err = ch.SendMessage(reply, randomUser(t, c).ID)
	require.NoError(t, err, "send reply")

	replies, err := ch.GetReplies(msg.ID, nil)
	require.NoError(t, err, "get replies")
	assert.Len(t, replies, 1)
}

func TestChannel_MarkRead(t *testing.T) {
}

func TestChannel_RemoveMembers(t *testing.T) {
	c := initClient(t)
	ch := initChannel(t, c)
	defer func() {
		_ = ch.Delete()
	}()

	user := randomUser(t, c)
	err := ch.RemoveMembers(
		[]string{user.ID},
		&Message{Text: "some members", User: &User{ID: user.ID}},
	)

	require.NoError(t, err, "remove members")

	for _, member := range ch.Members {
		assert.NotEqual(t, member.User.ID, user.ID, "member is not present")
	}
}

func TestChannel_SendEvent(t *testing.T) {
}

func TestChannel_SendMessage(t *testing.T) {
	c := initClient(t)
	ch := initChannel(t, c)
	defer func() {
		_ = ch.Delete()
	}()

	user1 := randomUser(t, c)
	user2 := randomUser(t, c)
	msg := &Message{
		Text: "test message",
		User: user1,
	}

	msg, err := ch.SendMessage(msg, user2.ID)
	require.NoError(t, err, "send message")
	// check that message was updated
	assert.NotEmpty(t, msg.ID, "message has ID")
	assert.NotEmpty(t, msg.HTML, "message has HTML body")

	msg2 := &Message{
		Text:   "text message 2",
		User:   user1,
		Silent: true,
	}
	msg2, err = ch.SendMessage(msg2, user2.ID)
	require.NoError(t, err, "send message 2")
	// check that message was updated
	assert.NotEmpty(t, msg2.ID, "message has ID")
	assert.NotEmpty(t, msg2.HTML, "message has HTML body")
	assert.True(t, msg2.Silent, "message silent flag is set")
}

func TestChannel_Truncate(t *testing.T) {
	c := initClient(t)
	ch := initChannel(t, c)
	defer func() {
		_ = ch.Delete()
	}()

	user := randomUser(t, c)
	msg := &Message{
		Text: "test message",
		User: user,
	}
	msg, err := ch.SendMessage(msg, user.ID)
	require.NoError(t, err, "send message")

	// refresh channel state
	require.NoError(t, ch.refresh(), "refresh channel")

	assert.Equal(t, ch.Messages[0].ID, msg.ID, "message exists")

	err = ch.Truncate()
	require.NoError(t, err, "truncate channel")

	// refresh channel state
	require.NoError(t, ch.refresh(), "refresh channel")

	assert.Empty(t, ch.Messages, "message not exists")
}

func TestChannel_Update(t *testing.T) {
	c := initClient(t)
	ch := initChannel(t, c)

	err := ch.Update(map[string]interface{}{"color": "blue"},
		&Message{Text: "color is blue", User: &User{ID: randomUser(t, c).ID}})
	require.NoError(t, err)
}

func TestChannel_PartialUpdate(t *testing.T) {
	c := initClient(t)
	users := randomUsers(t, c, 5)

	members := make([]string, 0, len(users))
	for i := range users {
		members = append(members, users[i].ID)
	}

	var ch *Channel
	ch, err := c.CreateChannel("team", randomString(12), randomUser(t, c).ID, map[string]interface{}{
		"members": members,
		"color":   "blue",
		"age":     30,
	})
	require.NoError(t, err)
	err = ch.PartialUpdate(PartialUpdate{
		Set: map[string]interface{}{
			"color": "red",
		},
		Unset: []string{"age"},
	})
	require.NoError(t, err)
	err = ch.refresh()
	require.NoError(t, err)
	require.Equal(t, "red", ch.ExtraData["color"])
	require.Equal(t, nil, ch.ExtraData["age"])
}

func TestChannel_AddModerators(t *testing.T) {
}

func TestChannel_DemoteModerators(t *testing.T) {
}

func TestChannel_UnBanUser(t *testing.T) {
}

func TestChannel_SendFile(t *testing.T) {
	c := initClient(t)
	ch := initChannel(t, c)

	var url string

	t.Run("Send file", func(t *testing.T) {
		file, err := os.Open(path.Join("testdata", "helloworld.txt"))
		if err != nil {
			t.Fatal(err)
		}

		url, err = ch.SendFile(SendFileRequest{
			Reader:   file,
			FileName: "HelloWorld.txt",
			User:     randomUser(t, c),
		})
		if err != nil {
			t.Fatalf("send file failed: %s", err)
		}
		if url == "" {
			t.Fatal("upload file returned empty url")
		}
	})

	t.Run("Delete file", func(t *testing.T) {
		err := ch.DeleteFile(url)
		if err != nil {
			t.Fatalf("delete file failed: %s", err.Error())
		}
	})
}

func TestChannel_SendImage(t *testing.T) {
	c := initClient(t)
	ch := initChannel(t, c)

	var url string

	t.Run("Send image", func(t *testing.T) {
		file, err := os.Open(path.Join("testdata", "helloworld.jpg"))
		if err != nil {
			t.Fatal(err)
		}

		url, err = ch.SendImage(SendFileRequest{
			Reader:      file,
			FileName:    "HelloWorld.jpg",
			User:        randomUser(t, c),
			ContentType: "image/jpeg",
		})

		if err != nil {
			t.Fatalf("Send image failed: %s", err.Error())
		}

		if url == "" {
			t.Fatal("upload image returned empty url")
		}
	})

	t.Run("Delete image", func(t *testing.T) {
		err := ch.DeleteImage(url)
		if err != nil {
			t.Fatalf("delete image failed: %s", err.Error())
		}
	})
}

func TestChannel_AcceptInvite(t *testing.T) {
	c := initClient(t)

	users := randomUsers(t, c, 5)

	members := make([]string, 0, len(users))
	for i := range users {
		members = append(members, users[i].ID)
	}

	ch, err := c.CreateChannel("team", randomString(12), randomUser(t, c).ID, map[string]interface{}{
		"members": members,
		"invites": []string{members[0]},
	})

	require.NoError(t, err, "create channel")
	err = ch.AcceptInvite(members[0], &Message{Text: "accepted", User: &User{ID: members[0]}})
	require.NoError(t, err, "accept invite")
}

func TestChannel_RejectInvite(t *testing.T) {
	c := initClient(t)

	users := randomUsers(t, c, 5)

	members := make([]string, 0, len(users))
	for i := range users {
		members = append(members, users[i].ID)
	}

	ch, err := c.CreateChannel("team", randomString(12), randomUser(t, c).ID, map[string]interface{}{
		"members": members,
		"invites": []string{members[0]},
	})

	require.NoError(t, err, "create channel")
	err = ch.RejectInvite(members[0], &Message{Text: "rejected", User: &User{ID: members[0]}})
	require.NoError(t, err, "reject invite")
}

func TestChannel_Mute_Unmute(t *testing.T) {
	c := initClient(t)

	users := randomUsers(t, c, 5)

	members := make([]string, 0, len(users))
	for i := range users {
		members = append(members, users[i].ID)
	}

	ch, err := c.CreateChannel("messaging", randomString(12), randomUser(t, c).ID, map[string]interface{}{
		"members": members,
	})
	require.NoError(t, err, "create channel")

	// mute the channel
	mute, err := ch.Mute(members[0], nil)
	require.NoError(t, err, "mute channel")

	require.Equal(t, ch.CID, mute.ChannelMute.Channel.CID)
	require.Equal(t, members[0], mute.ChannelMute.User.ID)
	// query for muted the channel
	channels, err := c.QueryChannels(&QueryOption{
		UserID: members[0],
		Filter: map[string]interface{}{
			"muted": true,
			"cid":   ch.CID,
		},
	})
	require.NoError(t, err, "query muted channel")
	require.Len(t, channels, 1)
	require.Equal(t, channels[0].CID, ch.CID)

	// unmute the channel
	err = ch.Unmute(members[0])
	require.NoError(t, err, "mute channel")

	// query for unmuted the channel should return 1 results
	channels, err = c.QueryChannels(&QueryOption{
		UserID: members[0],
		Filter: map[string]interface{}{
			"muted": false,
			"cid":   ch.CID,
		},
	})
	require.NoError(t, err, "query muted channel")
	require.Len(t, channels, 1)
}

func ExampleChannel_Update() {
	// https://getstream.io/chat/docs/channel_permissions/?language=python
	client := &Client{}

	data := map[string]interface{}{
		"image":      "https://path/to/image",
		"created_by": "elon",
		"roles":      map[string]string{"elon": "admin", "gwynne": "moderator"},
	}

	spacexChannel := client.Channel("team", "spacex")
	if err := spacexChannel.Update(data, nil); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func (c *Client) ExampleClient_CreateChannel() {
	client, _ := NewClient("XXXX", "XXXX")

	channel, _ := client.CreateChannel("team", "stream", "tommaso", nil)
	_, _ = channel.SendMessage(&Message{
		User: &User{ID: "tomosso"},
		Text: "hi there!",
	}, "tomosso")
}
