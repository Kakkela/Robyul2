package plugins

import (
	"strings"
	"time"

	"fmt"

	"github.com/Seklfreak/Robyul2/cache"
	"github.com/Seklfreak/Robyul2/helpers"
	"github.com/Seklfreak/Robyul2/models"
	"github.com/Seklfreak/Robyul2/shardmanager"
	"github.com/bwmarrin/discordgo"
	"github.com/globalsign/mgo/bson"
	"github.com/olebedev/when"
	"github.com/olebedev/when/rules/common"
	"github.com/olebedev/when/rules/en"
)

type Reminders struct {
	parser *when.Parser
}

// maps guildid => custom message
var customReminderMsgMap map[string]string

func (r *Reminders) Commands() []string {
	return []string{
		"remind",
		"remindme",
		"rm",
		"reminders",
		"rms",
	}
}

func (r *Reminders) Init(session *shardmanager.Manager) {
	r.parser = when.New(nil)
	r.parser.Add(en.All...)
	r.parser.Add(common.All...)

	go func() {
		defer helpers.Recover()

		for {
			reminderBucket := make([]models.RemindersEntry, 0)
			err := helpers.MDbIterWithoutLogging(helpers.MdbCollection(models.RemindersTable).Find(nil)).All(&reminderBucket)
			if err != nil {
				helpers.RelaxLog(err)
				time.Sleep(10 * time.Second)
				continue
			}

			for _, reminders := range reminderBucket {
				changes := false

				// Downward loop for in-loop element removal
				for idx := len(reminders.Reminders) - 1; idx >= 0; idx-- {
					reminder := reminders.Reminders[idx]

					if reminder.Timestamp <= time.Now().Unix() {
						dmChannel, err := session.Session(0).UserChannelCreate(reminders.UserID)
						if err == nil {
							content := ":alarm_clock: You wanted me to remind you about this:\n" + "```" + helpers.ZERO_WIDTH_SPACE + reminder.Message + "```"
							if reminder.Message == "" {
								content = ":alarm_clock: You wanted me to remind you about something, but you didn't tell me about what. <:blobthinking:317028940885524490>"
							}

							helpers.SendMessage(
								dmChannel.ID,
								content,
							)
						}

						reminders.Reminders = append(reminders.Reminders[:idx], reminders.Reminders[idx+1:]...)
						changes = true
					}
				}

				if changes {
					err = helpers.MDbUpsertID(
						models.RemindersTable,
						reminders.ID,
						reminders,
					)
					helpers.RelaxLog(err)
				}
			}

			time.Sleep(5 * time.Second)
		}
	}()

	// Setup custom reminder messages.
	//  Could eventually be loaded from a db if we wanted guilds to set up there own. not an important enough plugin to need that atm
	customReminderMsgMap = map[string]string{
		"339227598544568340": "Ok I'll remind you at `%s` <:nayoungok:424683077793611777>", // nayoung cord
		"403003926720413699": "Ok I'll remind you at `%s` <:nayoungok:424683077793611777>", // snakeyesz dev
		"208673735580844032": "Ok I'll remind you at `%s` <:nayoungok:424683077793611777>", // sekl dev
	}

	cache.GetLogger().WithField("module", "reminders").Info("Started reminder loop (10s)")
}

func (r *Reminders) Action(command string, content string, msg *discordgo.Message, session *discordgo.Session) {
	if !helpers.ModuleIsAllowed(msg.ChannelID, msg.ID, msg.Author.ID, helpers.ModulePermReminders) {
		return
	}

	switch command {
	case "rm", "remind", "remindme":
		session.ChannelTyping(msg.ChannelID)

		channel, err := helpers.GetChannel(msg.ChannelID)
		helpers.Relax(err)

		parts := strings.Fields(content)

		if len(parts) < 3 {
			helpers.SendMessage(msg.ChannelID, ":x: Please check if the format is correct")
			return
		}

		r, err := r.parser.Parse(content, time.Now())
		helpers.Relax(err)
		if r == nil {
			helpers.SendMessage(msg.ChannelID, ":x: Please check if the format is correct")
			return
		}

		reminders := getReminders(msg.Author.ID)
		reminders.Reminders = append(reminders.Reminders, models.RemindersReminderEntry{
			Message:   strings.Replace(content, r.Text, "", 1),
			ChannelID: channel.ID,
			GuildID:   channel.GuildID,
			Timestamp: r.Time.Unix(),
		})

		err = helpers.MDbUpsertID(
			models.RemindersTable,
			reminders.ID,
			reminders,
		)
		helpers.Relax(err)

		var userLocation *time.Location
		userData, err := helpers.GetUserUserdata(msg.Author.ID)
		if err == nil {
			userLocation, _ = time.LoadLocation(userData.Timezone)
		}
		if userLocation == nil {
			userLocation, _ = time.LoadLocation("UTC")
		}

		// Check if guild has a custom message set
		if customMsg, ok := customReminderMsgMap[channel.GuildID]; ok {
			helpers.SendMessage(msg.ChannelID, fmt.Sprintf(customMsg, r.Time.In(userLocation).Format(time.UnixDate)))
		} else {
			helpers.SendMessage(msg.ChannelID, "Ok I'll remind you at `"+r.Time.In(userLocation).Format(time.UnixDate)+" ` <:blobokhand:317032017164238848>")
		}
		break

	case "rms", "reminders": // TODO: better interface
		session.ChannelTyping(msg.ChannelID)

		reminders := getReminders(msg.Author.ID)
		var embedFields []*discordgo.MessageEmbedField

		var userLocation *time.Location
		userData, err := helpers.GetUserUserdata(msg.Author.ID)
		if err == nil {
			userLocation, _ = time.LoadLocation(userData.Timezone)
		}
		if userLocation == nil {
			userLocation, _ = time.LoadLocation("UTC")
		}

		for _, reminder := range reminders.Reminders {
			ts := time.Unix(reminder.Timestamp, 0)

			embedFields = append(embedFields, &discordgo.MessageEmbedField{
				Inline: false,
				Name:   "At " + ts.In(userLocation).Format(time.UnixDate),
				Value:  reminder.Message,
			})
		}

		if len(embedFields) == 0 {
			helpers.SendMessage(msg.ChannelID, helpers.GetText("plugins.reminders.empty"))
			return
		}

		helpers.SendEmbed(msg.ChannelID, &discordgo.MessageEmbed{
			Title:  "Pending reminders",
			Fields: embedFields,
			Color:  0x0FADED,
		})
		break
	}
}

func getReminders(userID string) (reminder models.RemindersEntry) {
	err := helpers.MdbOne(
		helpers.MdbCollection(models.RemindersTable).Find(bson.M{"userid": userID}),
		&reminder,
	)

	// If user has no DB entries create an empty document
	if helpers.IsMdbNotFound(err) {
		err = helpers.MDbUpsert(
			models.RemindersTable,
			bson.M{"userid": userID},
			models.RemindersEntry{
				UserID:    userID,
				Reminders: make([]models.RemindersReminderEntry, 0),
			},
		)
		// If the creation was successful read the document
		if err != nil {
			panic(err)
		} else {
			return getReminders(userID)
		}
	} else if err != nil {
		panic(err)
	}

	return reminder
}
