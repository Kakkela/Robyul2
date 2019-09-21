package plugins

import (
	"net/url"
	"strconv"

	"strings"

	"github.com/Jeffail/gabs"
	"github.com/Seklfreak/Robyul2/helpers"
	"github.com/Seklfreak/Robyul2/shardmanager"
	"github.com/bwmarrin/discordgo"
)

type UrbanDict struct{}

func (u *UrbanDict) Commands() []string {
	return []string{
		"urban",
		"ub",
	}
}

func (u *UrbanDict) Init(session *shardmanager.Manager) {

}

func (u *UrbanDict) Action(command string, content string, msg *discordgo.Message, session *discordgo.Session) {
	if !helpers.ModuleIsAllowed(msg.ChannelID, msg.ID, msg.Author.ID, helpers.ModulePermUrban) {
		return
	}

	session.ChannelTyping(msg.ChannelID)

	if content == "" {
		helpers.SendMessage(msg.ChannelID, "You should pass a word to define <:blobthinking:317028940885524490>")
		return
	}

	endpoint := "https://api.urbandictionary.com/v0/define?term=" + url.QueryEscape(content)

	result, err := helpers.NetGetUAWithError(endpoint, helpers.DEFAULT_UA)
	helpers.Relax(err)
	json, err := gabs.ParseJSON(result)
	helpers.Relax(err)

	res, err := json.Path("list").Children()
	helpers.Relax(err)

	if len(res) == 0 {
		_, err = helpers.SendMessage(msg.ChannelID, "No results <:blobneutral:317029459720929281>")
		helpers.RelaxMessage(err, msg.ChannelID, msg.ID)
		return
	}

	object, e := res[0].ChildrenMap()
	helpers.Relax(e)

	description := object["definition"].Data().(string)
	description = strings.Replace(description, "["+strings.Title(object["word"].Data().(string))+"]", "**"+strings.Title(object["word"].Data().(string))+"**", -1)
	description = strings.Replace(description, "["+object["word"].Data().(string)+"]", "**"+object["word"].Data().(string)+"**", -1)
	description = strings.Replace(description, "[", "", -1)
	description = strings.Replace(description, "]", "", -1)
	if len(description) > 1000 {
		description = description[:998] + " …"
	}

	example := object["example"].Data().(string)
	example = strings.Replace(example, "["+strings.Title(object["word"].Data().(string))+"]", "**"+strings.Title(object["word"].Data().(string))+"**", -1)
	example = strings.Replace(example, "["+object["word"].Data().(string)+"]", "**"+object["word"].Data().(string)+"**", -1)
	example = strings.Replace(example, "[", "", -1)
	example = strings.Replace(example, "]", "", -1)
	if len(example) > 250 {
		example = example[:248] + " …"
	}

	definitionEmbed := &discordgo.MessageEmbed{
		Color:       0x134FE6,
		Title:       object["word"].Data().(string),
		Description: description,
		URL:         object["permalink"].Data().(string),
		Fields:      []*discordgo.MessageEmbedField{},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "powered by urbandictionary.com",
		},
	}

	if example != "" {
		definitionEmbed.Fields = append(definitionEmbed.Fields, &discordgo.MessageEmbedField{Name: "Example(s)", Value: example, Inline: false})
	}
	if strconv.FormatFloat(object["thumbs_up"].Data().(float64), 'f', 0, 64) != "0" || strconv.FormatFloat(object["thumbs_down"].Data().(float64), 'f', 0, 64) != "0" {
		definitionEmbed.Fields = append(definitionEmbed.Fields, &discordgo.MessageEmbedField{
			Name: "Votes",
			Value: ":+1: " + strconv.FormatFloat(object["thumbs_up"].Data().(float64), 'f', 0, 64) +
				" | :-1: " + strconv.FormatFloat(object["thumbs_down"].Data().(float64), 'f', 0, 64),
			Inline: true,
		})
	}

	_, err = helpers.SendEmbed(msg.ChannelID, definitionEmbed)
	helpers.RelaxEmbed(err, msg.ChannelID, msg.ID)
}
