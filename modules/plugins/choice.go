package plugins

import (
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"

	"github.com/Seklfreak/Robyul2/helpers"
	"github.com/bwmarrin/discordgo"
)

type Choice struct{}

func (c *Choice) Commands() []string {
	return []string{
		"choose",
		"choice",
		"roll",
	}
}

var (
	splitChooseRegex *regexp.Regexp
)

func (c *Choice) Init(session *discordgo.Session) {
	splitChooseRegex = regexp.MustCompile(`'.*?'|".*?"|\S+`)
}

func (c *Choice) Action(command string, content string, msg *discordgo.Message, session *discordgo.Session) {
	if !helpers.ModuleIsAllowed(msg.ChannelID, msg.ID, msg.Author.ID, helpers.ModulePermChoice) {
		return
	}

	switch command {
	case "choose", "choice": // [p]choose <option a> <option b> [...]
		choices := splitChooseRegex.FindAllString(content, -1)

		if len(choices) <= 1 {
			_, err := helpers.SendMessage(msg.ChannelID, helpers.GetText("bot.arguments.too-few"))
			helpers.Relax(err)
			return
		}

		choice := choices[rand.Intn(len(choices))]
		choice = strings.Trim(choice, "\"")
		choice = strings.Trim(choice, "\"")

		_, err := helpers.SendMessage(msg.ChannelID, "I've chosen `"+choice+"` <a:ablobsmile:393869335312990209>")
		helpers.Relax(err)
		return
	case "roll": // [p]roll [<max numb, default: 100>]
		var err error
		maxN := 100
		if content != "" {
			maxN, err = strconv.Atoi(content)
			if err != nil || maxN < 1 {
				_, err := helpers.SendMessage(msg.ChannelID, helpers.GetText("bot.arguments.invalid"))
				helpers.Relax(err)
				return
			}
		}
		_, err = helpers.SendMessage(msg.ChannelID, fmt.Sprintf("<@%s> :game_die: %d :game_die:", msg.Author.ID, rand.Intn(maxN)+1))
		helpers.Relax(err)
		return
	}
}
