package modules

import (
	"github.com/Seklfreak/Robyul2/modules/plugins"
	"github.com/Seklfreak/Robyul2/modules/plugins/biasgame"
	"github.com/Seklfreak/Robyul2/modules/plugins/eventlog"
	"github.com/Seklfreak/Robyul2/modules/plugins/idols"
	"github.com/Seklfreak/Robyul2/modules/plugins/levels"
	"github.com/Seklfreak/Robyul2/modules/plugins/mod"
	"github.com/Seklfreak/Robyul2/modules/plugins/notifications"
	"github.com/Seklfreak/Robyul2/modules/plugins/nugugame"
	"github.com/Seklfreak/Robyul2/modules/plugins/youtube"
)

var (
	pluginCache         map[string]*Plugin
	extendedPluginCache map[string]*ExtendedPlugin

	PluginList = []Plugin{
		&notifications.Handler{},
		&plugins.About{},
		&plugins.Stats{},
		&plugins.Uptime{},
		&plugins.Translator{},
		&plugins.UrbanDict{},
		&plugins.Weather{},
		&plugins.VLive{},
		// &instagram.Handler{},
		// &plugins.Facebook{},
		&plugins.WolframAlpha{},
		&plugins.LastFm{},
		&plugins.Twitch{},
		&plugins.Charts{},
		&plugins.Choice{},
		&plugins.Osu{},
		&plugins.Reminders{},
		&plugins.Ratelimit{},
		&plugins.Gfycat{},
		&plugins.RandomPictures{},
		&youtube.Handler{},
		&plugins.Spoiler{},
		&plugins.RandomCat{},
		&plugins.RPS{},
		&plugins.Nuke{},
		&plugins.Dig{},
		&plugins.Streamable{},
		&plugins.Lyrics{},
		&plugins.Friend{},
		&plugins.Names{},
		&plugins.Reddit{},
		&plugins.Color{},
		&plugins.Dog{},
		&plugins.Debug{},
		&plugins.Donators{},
		&plugins.Ping{},
		//&google.Handler{},
		&plugins.BotStatus{},
		&plugins.VanityInvite{},
		&plugins.DiscordMoney{},
		&plugins.Whois{},
		&plugins.Isup{},
		&plugins.ModulePermissions{},
		&plugins.M8ball{},
		&plugins.Feedback{},
		&plugins.DM{},
		&plugins.EmbedPost{},
		&plugins.Useruploads{},
		&plugins.Move{},
		&plugins.Crypto{},
		&plugins.Imgur{},
		&plugins.Steam{},
		&plugins.Config{},
		&plugins.Storage{},
		&plugins.Mirror{},
	}

	PluginExtendedList = []ExtendedPlugin{
		&plugins.Bias{},
		&plugins.GuildAnnouncements{},
		&levels.Levels{},
		&plugins.Gallery{},
		&plugins.CustomCommands{},
		&plugins.ReactionPolls{},
		&mod.Mod{},
		&plugins.AutoRoles{},
		&plugins.Starboard{},
		&plugins.Autoleaver{},
		&plugins.Persistency{},
		&plugins.Twitter{},
		&eventlog.Handler{},
		&plugins.Perspective{},
		&biasgame.Module{},
		&nugugame.Module{},
		&idols.Module{},
	}
)
