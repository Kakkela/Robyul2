package plugins

import (
	"strconv"

	"github.com/Seklfreak/Robyul2/helpers"
	"github.com/Seklfreak/Robyul2/ratelimits"
	"github.com/Seklfreak/Robyul2/shardmanager"
	"github.com/bwmarrin/discordgo"
)

type Ratelimit struct{}

func (r *Ratelimit) Commands() []string {
	return []string{
		"limits",
	}
}

func (r *Ratelimit) Init(session *shardmanager.Manager) {

}

func (r *Ratelimit) Action(command string, content string, msg *discordgo.Message, session *discordgo.Session) {
	helpers.SendMessage(
		msg.ChannelID,
		"You've still got "+strconv.Itoa(int(ratelimits.Container.Get(msg.Author.ID)))+" commands left",
	)
}
