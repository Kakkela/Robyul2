package plugins

import (
	"fmt"
	"regexp"
	"strings"

	"time"

	"github.com/Seklfreak/Robyul2/cache"
	"github.com/Seklfreak/Robyul2/helpers"
	"github.com/Seklfreak/Robyul2/metrics"
	"github.com/Seklfreak/Robyul2/models"
	"github.com/Seklfreak/Robyul2/shardmanager"
	"github.com/bwmarrin/discordgo"
	"github.com/globalsign/mgo/bson"
	"github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack"
)

type Gallery struct{}

func (g *Gallery) Commands() []string {
	return []string{
		"gallery",
	}
}

const (
	galleryUrlRegexText = `(<?https?:\/\/[^\s]+>?)`
)

var (
	galleryUrlRegex *regexp.Regexp
	galleries       []models.GalleryEntry
)

func (g *Gallery) Init(session *shardmanager.Manager) {
	galleryUrlRegex = regexp.MustCompile(galleryUrlRegexText)
	var err error
	galleries, err = g.GetGalleries()
	helpers.Relax(err)
}

func (g *Gallery) Uninit(session *shardmanager.Manager) {

}

func (g *Gallery) Action(command string, content string, msg *discordgo.Message, session *discordgo.Session) {
	if !helpers.ModuleIsAllowed(msg.ChannelID, msg.ID, msg.Author.ID, helpers.ModulePermGallery) {
		return
	}

	args := strings.Fields(content)
	if len(args) >= 1 {
		switch args[0] {
		case "add": // [p]gallery add <source channel> <target channel>
			helpers.RequireMod(msg, func() {
				if len(args) < 3 {
					helpers.SendMessage(msg.ChannelID, helpers.GetText("bot.arguments.too-few"))
					return
				}

				session.ChannelTyping(msg.ChannelID)

				channel, err := helpers.GetChannel(msg.ChannelID)
				helpers.Relax(err)
				guild, err := helpers.GetGuild(channel.GuildID)
				helpers.Relax(err)
				sourceChannel, err := helpers.GetChannelFromMention(msg, args[1])
				if err != nil || sourceChannel.ID == "" || sourceChannel.GuildID != channel.GuildID {
					helpers.SendMessage(msg.ChannelID, helpers.GetText("bot.arguments.invalid"))
					return
				}
				targetChannel, err := helpers.GetChannelFromMention(msg, args[2])
				if err != nil || targetChannel.ID == "" || targetChannel.GuildID != channel.GuildID {
					helpers.SendMessage(msg.ChannelID, helpers.GetText("bot.arguments.invalid"))
					return
				}

				newID, err := helpers.MDbInsert(models.GalleryTable, models.GalleryEntry{
					SourceChannelID: sourceChannel.ID,
					TargetChannelID: targetChannel.ID,
					GuildID:         channel.GuildID,
					AddedByUserID:   msg.Author.ID,
				})
				helpers.Relax(err)

				_, err = helpers.EventlogLog(time.Now(), channel.GuildID, helpers.MdbIdToHuman(newID),
					models.EventlogTargetTypeRobyulGallery, msg.Author.ID,
					models.EventlogTypeRobyulGalleryAdd, "",
					nil,
					[]models.ElasticEventlogOption{
						{
							Key:   "gallery_sourcechannelid",
							Value: sourceChannel.ID,
							Type:  models.EventlogTargetTypeChannel,
						},
						{
							Key:   "gallery_targetchannelid",
							Value: targetChannel.ID,
							Type:  models.EventlogTargetTypeChannel,
						},
					}, false)
				helpers.RelaxLog(err)

				cache.GetLogger().WithField("module", "galleries").Info(fmt.Sprintf("Added Gallery on Server %s (%s) posting from #%s (%s) to #%s (%s)",
					guild.Name, guild.ID, sourceChannel.Name, sourceChannel.ID, targetChannel.Name, targetChannel.ID))
				_, err = helpers.SendMessage(msg.ChannelID, helpers.GetText("plugins.gallery.add-success"))
				helpers.RelaxMessage(err, msg.ChannelID, msg.ID)

				galleries, err = g.GetGalleries()
				helpers.RelaxLog(err)
				return
			})
		case "list": // [p]gallery list
			session.ChannelTyping(msg.ChannelID)
			channel, err := helpers.GetChannel(msg.ChannelID)
			helpers.Relax(err)
			var entryBucket []models.GalleryEntry
			err = helpers.MDbIter(helpers.MdbCollection(models.GalleryTable).Find(bson.M{"guildid": channel.GuildID})).All(&entryBucket)
			helpers.Relax(err)

			if entryBucket == nil || len(entryBucket) <= 0 {
				helpers.SendMessage(msg.ChannelID, helpers.GetText("plugins.gallery.list-empty"))
				return
			}

			resultMessage := ":frame_photo: Galleries on this server:\n"
			for _, entry := range entryBucket {
				resultMessage += fmt.Sprintf("`%s`: posting from <#%s> to <#%s>\n",
					helpers.MdbIdToHuman(entry.ID), entry.SourceChannelID, entry.TargetChannelID)
			}
			resultMessage += fmt.Sprintf("Found **%d** Galleries in total.", len(entryBucket))

			_, err = helpers.SendMessage(msg.ChannelID, resultMessage)
			helpers.RelaxMessage(err, msg.ChannelID, msg.ID)
			return
		case "delete", "del", "remove": // [p]gallery delete <gallery id>
			helpers.RequireAdmin(msg, func() {
				session.ChannelTyping(msg.ChannelID)
				if len(args) < 2 {
					_, err := helpers.SendMessage(msg.ChannelID, helpers.GetText("bot.arguments.too-few"))
					helpers.Relax(err)
					return
				}

				channel, err := helpers.GetChannel(msg.ChannelID)
				helpers.Relax(err)

				var entryBucket models.GalleryEntry
				err = helpers.MdbOne(
					helpers.MdbCollection(models.GalleryTable).Find(bson.M{"guildid": channel.GuildID, "_id": helpers.HumanToMdbId(args[1])}),
					&entryBucket,
				)
				if helpers.IsMdbNotFound(err) {
					helpers.SendMessage(msg.ChannelID, helpers.GetText("plugins.gallery.delete-not-found"))
					return
				}
				helpers.Relax(err)

				err = helpers.MDbDelete(models.GalleryTable, entryBucket.ID)
				helpers.Relax(err)

				_, err = helpers.EventlogLog(time.Now(), entryBucket.GuildID, helpers.MdbIdToHuman(entryBucket.ID),
					models.EventlogTargetTypeRobyulGallery, msg.Author.ID,
					models.EventlogTypeRobyulGalleryRemove, "",
					nil,
					[]models.ElasticEventlogOption{
						{
							Key:   "gallery_sourcechannelid",
							Value: entryBucket.SourceChannelID,
						},
						{
							Key:   "gallery_targetchannelid",
							Value: entryBucket.TargetChannelID,
						},
					}, false)
				helpers.RelaxLog(err)

				cache.GetLogger().WithField("module", "galleries").Info(fmt.Sprintf("Deleted Gallery on Server #%s posting from #%s to #%s",
					channel.GuildID, entryBucket.SourceChannelID, entryBucket.TargetChannelID))
				_, err = helpers.SendMessage(msg.ChannelID, helpers.GetText("plugins.gallery.delete-success"))
				helpers.Relax(err)

				galleries, err = g.GetGalleries()
				helpers.RelaxLog(err)
				return
			})
		case "refresh": // [p]gallery refresh
			helpers.RequireBotAdmin(msg, func() {
				session.ChannelTyping(msg.ChannelID)
				var err error
				galleries, err = g.GetGalleries()
				helpers.RelaxLog(err)
				_, err = helpers.SendMessage(msg.ChannelID, helpers.GetText("plugins.gallery.refreshed-config"))
				helpers.Relax(err)
			})
		}
	}
}

func (g *Gallery) OnMessage(content string, msg *discordgo.Message, session *discordgo.Session) {
	go func() {
		defer helpers.Recover()
	TryNextGallery:
		for _, gallery := range galleries {
			if gallery.SourceChannelID == msg.ChannelID {
				// ignore bot messages
				if msg.Author.Bot == true {
					continue TryNextGallery
				}
				sourceChannel, err := helpers.GetChannel(msg.ChannelID)
				helpers.Relax(err)
				// ignore commands
				prefix := helpers.GetPrefixForServer(sourceChannel.GuildID)
				if prefix != "" {
					if strings.HasPrefix(content, prefix) {
						return
					}
				}
				var linksToRepost []string
				// get mirror attachements
				if len(msg.Attachments) > 0 {
					for _, attachement := range msg.Attachments {
						linksToRepost = append(linksToRepost, attachement.URL)
					}
				}
				// get mirror links
				if strings.Contains(msg.Content, "http") {
					linksFound := galleryUrlRegex.FindAllString(msg.Content, -1)
					if len(linksFound) > 0 {
						for _, linkFound := range linksFound {
							if strings.HasPrefix(linkFound, "<") == false && strings.HasSuffix(linkFound, ">") == false {
								linksToRepost = append(linksToRepost, linkFound)
							}
						}
					}
				}
				// check if we have target channel
				_, err = helpers.GetChannelWithoutApi(gallery.TargetChannelID)
				if err != nil {
					continue TryNextGallery
				}
				// get webhook
				webhook, err := helpers.GetWebhook(gallery.GuildID, gallery.TargetChannelID)
				if err != nil && !strings.Contains(err.Error(), "no permission to manage webhooks") {
					helpers.Relax(err)
				}
				// post mirror links
				if len(linksToRepost) > 0 {
					for _, linkToRepost := range linksToRepost {
						var newMessage *discordgo.Message
						if webhook != nil && webhook.ID != "" && webhook.Token != "" {
							newMessage, err = helpers.WebhookExecuteWithResult(
								webhook.ID,
								webhook.Token,
								&discordgo.WebhookParams{
									Content:   fmt.Sprintf("posted %s in <#%s>", linkToRepost, gallery.SourceChannelID),
									Username:  msg.Author.Username,
									AvatarURL: helpers.GetAvatarUrl(msg.Author),
								},
							)
							if err != nil {
								helpers.RelaxLog(err)
								continue
							}
						} else {
							newMessages, err := helpers.SendMessage(gallery.TargetChannelID,
								fmt.Sprintf("%s posted %s in <#%s>", msg.Author.Username, linkToRepost, gallery.SourceChannelID))
							if err != nil {
								helpers.RelaxLog(err)
								continue
							}
							newMessage = newMessages[0]
						}
						err = g.rememberPostedMessage(msg, newMessage)
						helpers.RelaxLog(err)
						metrics.GalleryPostsSent.Add(1)
					}
				}
			}
		}
	}()
}

type Gallery_PostedMessage struct {
	ChannelID string
	MessageID string
}

func (g *Gallery) rememberPostedMessage(sourceMessage *discordgo.Message, mirroredMessage *discordgo.Message) error {
	redis := cache.GetRedisClient()
	key := fmt.Sprintf("robyul2-discord:gallery:postedmessage:%s", sourceMessage.ID)

	item := new(Gallery_PostedMessage)
	item.ChannelID = mirroredMessage.ChannelID
	item.MessageID = mirroredMessage.ID

	itemBytes, err := msgpack.Marshal(&item)
	if err != nil {
		return err
	}

	_, err = redis.LPush(key, itemBytes).Result()
	if err != nil {
		return err
	}

	_, err = redis.Expire(key, time.Hour*1).Result()
	return err
}

func (g *Gallery) getRememberedMessages(sourceMessage *discordgo.Message) ([]Gallery_PostedMessage, error) {
	redis := cache.GetRedisClient()
	key := fmt.Sprintf("robyul2-discord:gallery:postedmessage:%s", sourceMessage.ID)

	length, err := redis.LLen(key).Result()
	if err != nil {
		return []Gallery_PostedMessage{}, err
	}

	if length <= 0 {
		return []Gallery_PostedMessage{}, err
	}

	result, err := redis.LRange(key, 0, length-1).Result()
	if err != nil {
		return []Gallery_PostedMessage{}, err
	}

	rememberedMessages := make([]Gallery_PostedMessage, 0)
	for _, messageData := range result {
		var message Gallery_PostedMessage
		err = msgpack.Unmarshal([]byte(messageData), &message)
		if err != nil {
			continue
		}
		rememberedMessages = append(rememberedMessages, message)
	}

	return rememberedMessages, nil
}

func (g *Gallery) OnGuildMemberAdd(member *discordgo.Member, session *discordgo.Session) {
}

func (g *Gallery) OnGuildMemberRemove(member *discordgo.Member, session *discordgo.Session) {
}

func (g *Gallery) GetGalleries() (galleries []models.GalleryEntry, err error) {
	err = helpers.MDbIterWithoutLogging(helpers.MdbCollection(models.GalleryTable).Find(nil)).All(&galleries)
	return
}

func (g *Gallery) OnReactionAdd(reaction *discordgo.MessageReactionAdd, session *discordgo.Session) {

}
func (g *Gallery) OnReactionRemove(reaction *discordgo.MessageReactionRemove, session *discordgo.Session) {

}
func (g *Gallery) OnGuildBanAdd(user *discordgo.GuildBanAdd, session *discordgo.Session) {

}
func (g *Gallery) OnGuildBanRemove(user *discordgo.GuildBanRemove, session *discordgo.Session) {

}
func (g *Gallery) OnMessageDelete(msg *discordgo.MessageDelete, session *discordgo.Session) {
	go func() {
		defer helpers.Recover()
		var err error
		var rememberedMessages []Gallery_PostedMessage

		for _, gallery := range galleries {
			if gallery.SourceChannelID == msg.ChannelID {
				rememberedMessages, err = g.getRememberedMessages(msg.Message)
				helpers.Relax(err)

				for _, messageData := range rememberedMessages {
					err = session.ChannelMessageDelete(messageData.ChannelID, messageData.MessageID)
					if err != nil {
						msgAuthorID := "N/A"
						if msg.Author != nil {
							msgAuthorID = msg.Author.ID
						}

						cache.GetLogger().WithFields(logrus.Fields{
							"module":            "gallery",
							"sourceChannelID":   msg.ChannelID,
							"sourceMessageID":   msg.ID,
							"sourceAuthorID":    msgAuthorID,
							"mirroredChannelID": messageData.ChannelID,
							"mirroredMessageID": messageData.MessageID,
						}).Warn(
							"Deleting mirrored message failed:", err.Error(),
						)
					}
				}
			}
		}
	}()
}
