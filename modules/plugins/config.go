package plugins

import (
	"strings"

	"fmt"

	"github.com/Seklfreak/Robyul2/cache"
	"github.com/Seklfreak/Robyul2/helpers"
	"github.com/Seklfreak/Robyul2/helpers/dgwidgets"
	"github.com/Seklfreak/Robyul2/shardmanager"
	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
)

type configAction func(args []string, in *discordgo.Message, out **discordgo.MessageSend) (next configAction)

type Config struct{}

func (m *Config) Commands() []string {
	return []string{
		"config",
	}
}

func (m *Config) Init(session *shardmanager.Manager) {
}

func (m *Config) Action(command string, content string, msg *discordgo.Message, session *discordgo.Session) {
	if !helpers.ModuleIsAllowed(msg.ChannelID, msg.ID, msg.Author.ID, helpers.ModulePermMod) {
		return
	}

	var result *discordgo.MessageSend
	args := strings.Fields(content)

	action := m.actionStart
	for action != nil {
		action = action(args, msg, &result)
	}
}

func (m *Config) actionStart(args []string, in *discordgo.Message, out **discordgo.MessageSend) configAction {
	cache.GetSession().SessionForGuildS(in.GuildID).ChannelTyping(in.ChannelID)

	if len(args) > 1 {
		switch args[0] {
		case "set":
			if len(args) < 2 {
				*out = m.newMsg(helpers.GetText("bot.arguments.too-few"))
				return m.actionFinish
			}
			switch args[1] {
			case "admin":
				return m.actionSetAdmin
			case "mod":
				return m.actionSetMod
			}
			break
		}
	}

	return m.actionStatus
}

// [p]config set admin <role name or id>
func (m *Config) actionSetAdmin(args []string, in *discordgo.Message, out **discordgo.MessageSend) configAction {
	if !helpers.IsAdmin(in) {
		*out = m.newMsg("admin.no_permission")
		return m.actionFinish
	}

	if len(args) < 3 {
		*out = m.newMsg(helpers.GetText("bot.arguments.too-few"))
		return m.actionFinish
	}

	channel, err := helpers.GetChannel(in.ChannelID)
	helpers.Relax(err)

	guild, err := helpers.GetGuild(channel.GuildID)
	helpers.Relax(err)

	// look for target role in guild roles
	var targetRole *discordgo.Role
	for _, guildRole := range guild.Roles {
		if guildRole.ID == args[2] || strings.ToLower(guildRole.Name) == strings.ToLower(args[2]) {
			targetRole = guildRole
		}
	}

	var roleAdded, roleRemoved bool

	adminRolesWithout := make([]string, 0)

	// get old guild config
	guildConfig := helpers.GuildSettingsGetCached(channel.GuildID)

	// should we remove the role?
	for _, existingAdminRoleID := range guildConfig.AdminRoleIDs {
		if targetRole == nil || targetRole.ID == "" {
			if existingAdminRoleID == args[2] {
				roleRemoved = true
			} else {
				adminRolesWithout = append(adminRolesWithout, existingAdminRoleID)
			}
		} else {
			if existingAdminRoleID == targetRole.ID {
				roleRemoved = true
			} else {
				adminRolesWithout = append(adminRolesWithout, existingAdminRoleID)
			}
		}
	}
	if roleRemoved {
		guildConfig.AdminRoleIDs = adminRolesWithout
	}

	// if no role removed, add role
	if !roleRemoved {
		if targetRole != nil && targetRole.ID != "" {
			roleAdded = true
			guildConfig.AdminRoleIDs = append(guildConfig.AdminRoleIDs, targetRole.ID)
		}
	}

	if !roleAdded && !roleRemoved {
		*out = m.newMsg("bot.arguments.invalid")
		return m.actionFinish
	}

	// save new guild config
	err = helpers.GuildSettingsSet(channel.GuildID, guildConfig)
	helpers.Relax(err)

	// TODO: eventlog

	if roleAdded {
		*out = m.newMsg("plugins.config.admin-role-added")
		return m.actionFinish
	}
	if roleRemoved {
		*out = m.newMsg("plugins.config.admin-role-removed")
		return m.actionFinish
	}
	return nil
}

// [p]config set mod <role name or id>
func (m *Config) actionSetMod(args []string, in *discordgo.Message, out **discordgo.MessageSend) configAction {
	if !helpers.IsAdmin(in) {
		*out = m.newMsg("admin.no_permission")
		return m.actionFinish
	}

	if len(args) < 3 {
		*out = m.newMsg(helpers.GetText("bot.arguments.too-few"))
		return m.actionFinish
	}

	channel, err := helpers.GetChannel(in.ChannelID)
	helpers.Relax(err)

	guild, err := helpers.GetGuild(channel.GuildID)
	helpers.Relax(err)

	// look for target role in guild roles
	var targetRole *discordgo.Role
	for _, guildRole := range guild.Roles {
		if guildRole.ID == args[2] || strings.ToLower(guildRole.Name) == strings.ToLower(args[2]) {
			targetRole = guildRole
		}
	}

	var roleAdded, roleRemoved bool

	modRolesWithout := make([]string, 0)

	// get old guild config
	guildConfig := helpers.GuildSettingsGetCached(channel.GuildID)

	// should we remove the role?
	for _, existingModRoleID := range guildConfig.ModRoleIDs {
		if targetRole == nil || targetRole.ID == "" {
			if existingModRoleID == args[2] {
				roleRemoved = true
			} else {
				modRolesWithout = append(modRolesWithout, existingModRoleID)
			}
		} else {
			if existingModRoleID == targetRole.ID {
				roleRemoved = true
			} else {
				modRolesWithout = append(modRolesWithout, existingModRoleID)
			}
		}
	}
	if roleRemoved {
		guildConfig.ModRoleIDs = modRolesWithout
	}

	// if no role removed, add role
	if !roleRemoved {
		if targetRole != nil && targetRole.ID != "" {
			roleAdded = true
			guildConfig.ModRoleIDs = append(guildConfig.ModRoleIDs, targetRole.ID)
		}
	}

	if !roleAdded && !roleRemoved {
		*out = m.newMsg("bot.arguments.invalid")
		return m.actionFinish
	}

	// save new guild config
	err = helpers.GuildSettingsSet(channel.GuildID, guildConfig)
	helpers.Relax(err)

	if roleAdded {
		*out = m.newMsg("plugins.config.mod-role-added")
		return m.actionFinish
	}
	if roleRemoved {
		*out = m.newMsg("plugins.config.mod-role-removed")
		return m.actionFinish
	}
	return nil
}

// [p]config
func (m *Config) actionStatus(args []string, in *discordgo.Message, out **discordgo.MessageSend) configAction {
	channel, err := helpers.GetChannel(in.ChannelID)
	helpers.Relax(err)

	targetGuild, err := helpers.GetGuild(channel.GuildID)
	helpers.Relax(err)

	if len(args) >= 1 {
		if helpers.IsRobyulMod(in.Author.ID) {
			targetGuild, err = helpers.GetGuild(args[0])
			helpers.Relax(err)
		}
	}

	if !helpers.IsModByID(targetGuild.ID, in.Author.ID) && !helpers.IsRobyulMod(in.Author.ID) {
		*out = m.newMsg("mod.no_permission")
		return m.actionFinish
	}

	guildConfig := helpers.GuildSettingsGetCached(targetGuild.ID)

	prefix := helpers.GetPrefixForServer(targetGuild.ID)

	pages := make([]*discordgo.MessageEmbed, 0)

	adminsText := "Role Name Matching"
	if guildConfig.AdminRoleIDs != nil && len(guildConfig.AdminRoleIDs) > 0 {
		adminsText = "Custom Roles, <@&"
		adminsText += strings.Join(guildConfig.AdminRoleIDs, ">, <@&")
		adminsText += ">"
	}

	modsText := "Role Name Matching"
	if guildConfig.ModRoleIDs != nil && len(guildConfig.ModRoleIDs) > 0 {
		modsText = "Custom Roles, <@&"
		modsText += strings.Join(guildConfig.ModRoleIDs, ">, <@&")
		modsText += ">"
	}

	inspectsText := "Disabled"
	if guildConfig.InspectsChannel != "" {
		inspectsText = "Enabled, in <#" + guildConfig.InspectsChannel + ">"
	}

	nukeText := "Disabled"
	if guildConfig.NukeIsParticipating {
		nukeText = "Enabled, log in <#" + guildConfig.NukeLogChannel + ">"
	}

	levelsText := "Level Up Notification: "
	if guildConfig.LevelsNotificationCode != "" {
		levelsText += "Enabled"
		if guildConfig.LevelsNotificationDeleteAfter > 0 {
			levelsText += fmt.Sprintf(", deleting after %d seconds", guildConfig.LevelsNotificationDeleteAfter)
		}
	} else {
		levelsText += "Disabled"
	}
	levelsText += "\nIgnored Users: "
	if guildConfig.LevelsIgnoredUserIDs == nil || len(guildConfig.LevelsIgnoredUserIDs) <= 0 {
		levelsText += "None"
	} else {
		levelsText += "<@"
		levelsText += strings.Join(guildConfig.LevelsIgnoredUserIDs, ">, <@")
		levelsText += ">"
	}
	levelsText += "\nIgnored Channels: "
	if guildConfig.LevelsIgnoredChannelIDs == nil || len(guildConfig.LevelsIgnoredChannelIDs) <= 0 {
		levelsText += "None"
	} else {
		levelsText += "<#"
		levelsText += strings.Join(guildConfig.LevelsIgnoredChannelIDs, ">, <#")
		levelsText += ">"
	}
	levelsText += fmt.Sprintf("\nMax Badges: %d", helpers.GetMaxBadgesForGuild(targetGuild.ID))

	var autoRolesText string
	if (guildConfig.AutoRoleIDs == nil || len(guildConfig.AutoRoleIDs) <= 0) &&
		(guildConfig.DelayedAutoRoles == nil || len(guildConfig.DelayedAutoRoles) <= 0) {
		autoRolesText += "None"
	} else {
		if guildConfig.AutoRoleIDs != nil && len(guildConfig.AutoRoleIDs) > 0 {
			autoRolesText += "<@&"
			autoRolesText += strings.Join(guildConfig.AutoRoleIDs, ">, <@&")
			autoRolesText += ">"
		}
		if guildConfig.DelayedAutoRoles != nil && len(guildConfig.DelayedAutoRoles) > 0 {
			if autoRolesText != "" {
				autoRolesText += ", "
			}
			for _, delayedAutoRole := range guildConfig.DelayedAutoRoles {
				autoRolesText += fmt.Sprintf("<@&%s> after %s",
					delayedAutoRole.RoleID, delayedAutoRole.Delay.String())
			}
		}
	}

	starboardText := "Disabled"
	if guildConfig.StarboardChannelID != "" {
		starboardText = "Enabled, in <#" + guildConfig.StarboardChannelID + ">"
	}

	chatlogText := "Enabled"
	if guildConfig.ChatlogDisabled {
		chatlogText = "Disabled"
	}

	eventlogText := "Disabled"
	if !guildConfig.EventlogDisabled {
		eventlogText = "Enabled"
		if guildConfig.EventlogChannelIDs != nil && len(guildConfig.EventlogChannelIDs) > 0 {
			eventlogText += ", log in <#"
			eventlogText += strings.Join(guildConfig.EventlogChannelIDs, ">, <#")
			eventlogText += ">"
		}
	}

	var persistencyText string
	if !guildConfig.PersistencyBiasEnabled && (guildConfig.PersistencyRoleIDs == nil || len(guildConfig.PersistencyRoleIDs) <= 0) {
		persistencyText += "Disabled"
	} else {
		if guildConfig.PersistencyBiasEnabled {
			persistencyText += "Enabled, for Bias Roles"
		}
		if guildConfig.PersistencyRoleIDs != nil && len(guildConfig.PersistencyRoleIDs) > 0 {
			if persistencyText == "" {
				persistencyText += "Enabled, for "
			} else {
				persistencyText += ", and "
			}
			persistencyText += "<@&"
			persistencyText += strings.Join(guildConfig.PersistencyRoleIDs, ">, <@&")
			persistencyText += ">"
		}
	}

	perspectiveText := "Disabled"
	if guildConfig.PerspectiveIsParticipating {
		perspectiveText = "Enabled, log in <#" + guildConfig.PerspectiveChannelID + ">"
	}

	customCommandsText := "Moderators can add commands"
	if guildConfig.CustomCommandsEveryoneCanAdd {
		customCommandsText = "Everyone can add commands"
	}
	if guildConfig.CustomCommandsAddRoleID != "" {
		customCommandsText = "<@&" + guildConfig.CustomCommandsAddRoleID + "> and Moderators can add commands"
	}

	// TODO: info if blacklisted, or limited guild

	pages = append(pages, &discordgo.MessageEmbed{
		Fields: []*discordgo.MessageEmbedField{
			{
				Name: "Prefix",
				Value: fmt.Sprintf("`%s`\n`@%s#%s set prefix <new prefix>`",
					prefix,
					cache.GetSession().SessionForGuildS(in.GuildID).State.User.Username, cache.GetSession().SessionForGuildS(in.GuildID).State.User.Discriminator),
			},
			{
				Name:  "Admins",
				Value: adminsText,
			},
			{
				Name:  "Mods",
				Value: modsText,
			},
			{
				Name:  "Auto Inspects",
				Value: inspectsText + fmt.Sprintf("\n`%sauto-inspects-channel <#channel or channel id>`", prefix),
			},
			{
				Name:  "Nuke",
				Value: nukeText + fmt.Sprintf("\n`%snuke participate <#channel or channel id>`", prefix),
			},
			{
				Name:  "Levels",
				Value: levelsText,
			},
			{
				Name:  "Auto Roles",
				Value: autoRolesText,
			},
			{
				Name:  "Starboard",
				Value: starboardText,
			},
			{
				Name:  "Chatlog",
				Value: chatlogText,
			},
			{
				Name:  "Eventlog",
				Value: eventlogText,
			},
			{
				Name:  "Persistency",
				Value: persistencyText,
			},
			{
				Name:  "Perspective",
				Value: perspectiveText,
			},
			{
				Name:  "Custom Commands",
				Value: customCommandsText,
			},
		},
	})

	for _, page := range pages {
		page.Title = "Robyul Config for " + targetGuild.Name
		page.Color = 0xFADED
		page.Footer = &discordgo.MessageEmbedFooter{
			Text: "Server #" + targetGuild.ID,
		}
		if targetGuild.Icon != "" {
			page.Footer.IconURL = discordgo.EndpointGuildIcon(targetGuild.ID, targetGuild.Icon)
		}
	}

	if len(pages) > 1 {
		p := dgwidgets.NewPaginator(in.GuildID, in.ChannelID, in.Author.ID)
		p.Add(pages...)
		p.Spawn()
	} else if len(pages) == 1 {
		*out = &discordgo.MessageSend{Embed: pages[0]}
		return m.actionFinish
	}

	return nil
}

func (m *Config) actionFinish(args []string, in *discordgo.Message, out **discordgo.MessageSend) configAction {
	_, err := helpers.SendComplex(in.ChannelID, *out)
	helpers.Relax(err)

	return nil
}

func (m *Config) newMsg(content string) *discordgo.MessageSend {
	return &discordgo.MessageSend{Content: helpers.GetText(content)}
}

func (m *Config) Relax(err error) {
	if err != nil {
		panic(err)
	}
}

func (m *Config) logger() *logrus.Entry {
	return cache.GetLogger().WithField("module", "config")
}
