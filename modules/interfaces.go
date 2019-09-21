package modules

import (
	"github.com/Seklfreak/Robyul2/shardmanager"
	"github.com/bwmarrin/discordgo"
)

type BaseModule interface{}

type Plugin interface {
	BaseModule

	Commands() []string

	Init(session *shardmanager.Manager)

	Action(
		command string,
		content string,
		msg *discordgo.Message,
		session *discordgo.Session,
	)
}

type ExtendedPlugin interface {
	BaseModule

	Commands() []string

	Init(session *shardmanager.Manager)

	Uninit(session *shardmanager.Manager)

	Action(
		command string,
		content string,
		msg *discordgo.Message,
		session *discordgo.Session,
	)

	OnMessage(
		content string,
		msg *discordgo.Message,
		session *discordgo.Session,
	)

	OnMessageDelete(
		msg *discordgo.MessageDelete,
		session *discordgo.Session,
	)

	OnGuildMemberAdd(
		member *discordgo.Member,
		session *discordgo.Session,
	)

	OnGuildMemberRemove(
		member *discordgo.Member,
		session *discordgo.Session,
	)

	OnReactionAdd(
		reaction *discordgo.MessageReactionAdd,
		session *discordgo.Session,
	)

	OnReactionRemove(
		reaction *discordgo.MessageReactionRemove,
		session *discordgo.Session,
	)

	OnGuildBanAdd(
		user *discordgo.GuildBanAdd,
		session *discordgo.Session,
	)

	OnGuildBanRemove(
		user *discordgo.GuildBanRemove,
		session *discordgo.Session,
	)
}
