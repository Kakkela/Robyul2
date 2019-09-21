package plugins

import (
	"fmt"
	"strconv"
	"time"

	"strings"

	"context"

	"github.com/Seklfreak/Robyul2/cache"
	"github.com/Seklfreak/Robyul2/helpers"
	"github.com/Seklfreak/Robyul2/shardmanager"
	"github.com/bwmarrin/discordgo"
)

type Ping struct{}

func (p *Ping) Commands() []string {
	return []string{
		"ping",
	}
}

var (
	pingMessage string
)

func (p *Ping) Init(session *shardmanager.Manager) {
	pingMessage = helpers.GetText("plugins.ping.message")
	session.AddHandler(p.OnMessage)
}

func (p *Ping) Action(command string, content string, msg *discordgo.Message, session *discordgo.Session) {
	if !helpers.ModuleIsAllowed(msg.ChannelID, msg.ID, msg.Author.ID, helpers.ModulePermPing) {
		return
	}

	_, err := helpers.SendMessage(msg.ChannelID, pingMessage+" ~ "+strconv.FormatInt(time.Now().UnixNano(), 10))
	helpers.RelaxMessage(err, msg.ChannelID, msg.ID)
}

func (p *Ping) OnMessage(session *discordgo.Session, message *discordgo.MessageCreate) {
	if message.Author.ID != session.State.User.ID {
		return
	}

	if !strings.HasPrefix(message.Content, pingMessage+" ~ ") {
		return
	}

	textUnixNano := strings.Replace(message.Content, pingMessage+" ~ ", "", 1)

	parsedUnixNano, err := strconv.ParseInt(textUnixNano, 10, 64)
	if err != nil {
		return
	}

	gatewayTaken := time.Duration(time.Now().UnixNano() - parsedUnixNano)

	text := strings.Replace(message.Content, " ~ "+textUnixNano, "", 1) + "\nGateway Latency (receive message): " + gatewayTaken.String()

	started := time.Now()
	helpers.EditMessage(message.ChannelID, message.ID, text)
	apiTaken := time.Since(started)

	text = text + "\nHTTP API Latency (edit message): " + apiTaken.String()

	started = time.Now()
	helpers.GetMDbSession().Ping()
	mongodbTaken := time.Since(started)
	text = text + "\nMongoDB Latency: " + mongodbTaken.String()

	started = time.Now()
	cache.GetRedisClient().Ping()
	redisTaken := time.Since(started)
	text = text + "\nRedis Latency: " + redisTaken.String()

	if helpers.GetConfig().Path("elasticsearch.url").Data().(string) != "" {
		started = time.Now()
		cache.GetElastic().Ping(helpers.GetConfig().Path("elasticsearch.url").Data().(string)).Do(context.Background())
		elasticTaken := time.Since(started)
		text = text + "\nElasticSearch Latency: " + elasticTaken.String()
	}

	text += fmt.Sprintf("\nShard: %d", cache.GetSession().ShardForGuild(message.GuildID))

	helpers.EditMessage(message.ChannelID, message.ID, text)
}
