package plugins

import (
    "fmt"
    "github.com/Seklfreak/Robyul2/helpers"
    "github.com/Seklfreak/Robyul2/metrics"
    "github.com/Seklfreak/Robyul2/version"
    "github.com/bwmarrin/discordgo"
    "github.com/dustin/go-humanize"
    "runtime"
    "strconv"
    "time"
    "strings"
    "github.com/bradfitz/slice"
    "github.com/Seklfreak/Robyul2/logger"
    rethink "github.com/gorethink/gorethink"
)

type Stats struct{}

func (s *Stats) Commands() []string {
    return []string{
        "stats",
        "serverinfo",
        "userinfo",
        "voicestats",
    }
}

var (
    voiceStatesWithTime []VoiceStateWithTime
)

type DB_VoiceTime struct {
    ID           string        `gorethink:"id,omitempty"`
    GuildID      string        `gorethink:"guildid"`
    ChannelID    string        `gorethink:"channelid"`
    UserID       string        `gorethink:"userid"`
    JoinTimeUtc  time.Time     `gorethink:"join_time_utc"`
    LeaveTimeUtc time.Time     `gorethink:"leave_time_utc"`
}

type VoiceStateWithTime struct {
    VoiceState  *discordgo.VoiceState
    JoinTimeUtc time.Time
}

func (s *Stats) Init(session *discordgo.Session) {
    go func() {
        defer helpers.Recover()

        var voiceStatesBefore []*discordgo.VoiceState
        var voiceStatesCurrently []*discordgo.VoiceState
        for {
            voiceStatesCurrently = []*discordgo.VoiceState{}
            // get for all vc users
            for _, guild := range session.State.Guilds {
                for _, voiceState := range guild.VoiceStates {
                    voiceStatesCurrently = append(voiceStatesCurrently, voiceState)
                    alreadyInVoiceStatesWithTime := false
                    for _, voiceStateWithTime := range voiceStatesWithTime {
                        if voiceState.UserID == voiceStateWithTime.VoiceState.UserID && voiceState.ChannelID == voiceStateWithTime.VoiceState.ChannelID {
                            alreadyInVoiceStatesWithTime = true
                        }
                    }
                    if alreadyInVoiceStatesWithTime == false {
                        voiceStatesWithTime = append(voiceStatesWithTime, VoiceStateWithTime{VoiceState: voiceState, JoinTimeUtc: time.Now().UTC()})
                    }
                }
            }
            // check who left since last check
            for _, voiceStateBefore := range voiceStatesBefore {
                userStillConnected := false
                voiceStateWithTimeIndex := -1
                for _, voiceStateCurrently := range voiceStatesCurrently {
                    if voiceStateCurrently.UserID == voiceStateBefore.UserID && voiceStateCurrently.ChannelID == voiceStateBefore.ChannelID {
                        userStillConnected = true
                    }
                }
                if userStillConnected == false {
                    for i, voiceStateWithTimeEntry := range voiceStatesWithTime {
                        if voiceStateBefore.UserID == voiceStateWithTimeEntry.VoiceState.UserID && voiceStateBefore.ChannelID == voiceStateWithTimeEntry.VoiceState.ChannelID {
                            voiceStateWithTimeIndex = i
                        }
                    }
                }
                if voiceStateWithTimeIndex != -1 {
                    channel, err := session.Channel(voiceStateBefore.ChannelID)
                    helpers.Relax(err)
                    newVoiceTime := s.getVoiceTimeEntryByOrCreateEmpty("id", "")
                    newVoiceTime.GuildID = channel.GuildID
                    newVoiceTime.ChannelID = channel.ID
                    newVoiceTime.UserID = voiceStateBefore.UserID
                    newVoiceTime.LeaveTimeUtc = time.Now().UTC()
                    newVoiceTime.JoinTimeUtc = voiceStatesWithTime[voiceStateWithTimeIndex].JoinTimeUtc
                    s.setVoiceTimeEntry(newVoiceTime)
                    voiceStatesWithTime = append(voiceStatesWithTime[:voiceStateWithTimeIndex], voiceStatesWithTime[voiceStateWithTimeIndex+1:]...)
                    logger.PLUGIN.L("stats", fmt.Sprintf("Saved Voice Session Length in DB for user #%s in channel #%s on server #%s",
                        newVoiceTime.UserID, newVoiceTime.ChannelID, newVoiceTime.GuildID))
                }
            }
            voiceStatesBefore = voiceStatesCurrently

            time.Sleep(3 * time.Second)
        }
    }()

    logger.PLUGIN.L("stats", "Started voice stats loop (3s)") // @TODO: increase time
}

func (s *Stats) Action(command string, content string, msg *discordgo.Message, session *discordgo.Session) {
    switch command {
    case "stats":
        // Count guilds, channels and users
        users := make(map[string]string)
        channels := 0
        guilds := session.State.Guilds

        for _, guild := range guilds {
            channels += len(guild.Channels)

            lastAfterMemberId := ""
            for {
                members, err := session.GuildMembers(guild.ID, lastAfterMemberId, 1000)
                if len(members) <= 0 {
                    break
                }
                lastAfterMemberId = members[len(members)-1].User.ID
                helpers.Relax(err)
                for _, u := range members {
                    users[u.User.ID] = u.User.Username
                }
            }
        }

        // Get RAM stats
        var ram runtime.MemStats
        runtime.ReadMemStats(&ram)

        // Get uptime
        bootTime, err := strconv.ParseInt(metrics.Uptime.String(), 10, 64)
        if err != nil {
            bootTime = 0
        }

        uptime := time.Now().Sub(time.Unix(bootTime, 0)).String()

        session.ChannelMessageSendEmbed(msg.ChannelID, &discordgo.MessageEmbed{
            Color: 0x0FADED,
            Thumbnail: &discordgo.MessageEmbedThumbnail{
                URL: fmt.Sprintf(
                    "https://cdn.discordapp.com/avatars/%s/%s.jpg",
                    session.State.User.ID,
                    session.State.User.Avatar,
                ),
            },
            Fields: []*discordgo.MessageEmbedField{
                // Build
                {Name: "Build Time", Value: version.BUILD_TIME, Inline: false},
                {Name: "Build System", Value: version.BUILD_USER + "@" + version.BUILD_HOST, Inline: false},

                // System
                {Name: "Bot Uptime", Value: uptime, Inline: true},
                {Name: "Bot Version", Value: version.BOT_VERSION, Inline: true},
                {Name: "GO Version", Value: runtime.Version(), Inline: true},

                // Bot
                {Name: "Used RAM", Value: humanize.Bytes(ram.Alloc) + "/" + humanize.Bytes(ram.Sys), Inline: true},
                {Name: "Collected garbage", Value: humanize.Bytes(ram.TotalAlloc), Inline: true},
                {Name: "Running coroutines", Value: strconv.Itoa(runtime.NumGoroutine()), Inline: true},

                // Discord
                {Name: "Connected servers", Value: strconv.Itoa(len(guilds)), Inline: true},
                {Name: "Watching channels", Value: strconv.Itoa(channels), Inline: true},
                {Name: "Users with access to me", Value: strconv.Itoa(len(users)), Inline: true},

                // Link
                {Name: "Want more stats and awesome graphs?", Value: "Visit my [datadog dashboard](https://p.datadoghq.com/sb/066f13da3-7607f827de)", Inline: false},
            },
        })
    case "serverinfo":
        session.ChannelTyping(msg.ChannelID)
        currentChannel, err := session.Channel(msg.ChannelID)
        helpers.Relax(err)
        guild, err := session.Guild(currentChannel.GuildID)
        helpers.Relax(err)
        users := make(map[string]string)
        lastAfterMemberId := ""
        for {
            members, err := session.GuildMembers(guild.ID, lastAfterMemberId, 1000)
            helpers.Relax(err)
            if len(members) <= 0 {
                break
            }

            lastAfterMemberId = members[len(members)-1].User.ID
            for _, u := range members {
                users[u.User.ID] = u.User.Username
            }
        }

        textChannels := 0
        voiceChannels := 0
        for _, channel := range guild.Channels {
            if channel.Type == "voice" {
                voiceChannels += 1
            } else if channel.Type == "text" {
                textChannels += 1
            }
        }
        online := 0
        for _, presence := range guild.Presences {
            if presence.Status == discordgo.StatusOnline || presence.Status == discordgo.StatusDoNotDisturb || presence.Status == discordgo.StatusIdle {
                online += 1
            }
        }

        createdAtTime := helpers.GetTimeFromSnowflake(guild.ID)

        owner, err := session.User(guild.OwnerID)
        helpers.Relax(err)
        member, err := session.GuildMember(guild.ID, guild.OwnerID)
        helpers.Relax(err)
        ownerText := fmt.Sprintf("%s#%s", owner.Username, owner.Discriminator)
        if member.Nick != "" {
            ownerText = fmt.Sprintf("%s#%s ~ %s", owner.Username, owner.Discriminator, member.Nick)
        }

        emoteText := "None"
        emoteN := 0
        for _, emote := range guild.Emojis {
            if emoteN == 0 {
                emoteText = fmt.Sprintf("`%s`", emote.Name)
            } else {

                emoteText += fmt.Sprintf(", `%s`", emote.Name)
            }
            emoteN += 1
        }
        if emoteText != "None" {
            emoteText += fmt.Sprintf(" (%d in Total)", emoteN)
        }

        serverinfoEmbed := &discordgo.MessageEmbed{
            Color:       0x0FADED,
            Title:       guild.Name,
            Description: fmt.Sprintf("Since: %s. That's %s.", createdAtTime.Format(time.ANSIC), helpers.SinceInDaysText(createdAtTime)),
            Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Server ID: %s", guild.ID)},
            Fields: []*discordgo.MessageEmbedField{
                {Name: "Region", Value: guild.Region, Inline: true},
                {Name: "Users", Value: fmt.Sprintf("%d/%d", online, len(users)), Inline: true},
                {Name: "Text Channels", Value: strconv.Itoa(textChannels), Inline: true},
                {Name: "Voice Channels", Value: strconv.Itoa(voiceChannels), Inline: true},
                {Name: "Roles", Value: strconv.Itoa(len(guild.Roles)), Inline: true},
                {Name: "Owner", Value: ownerText, Inline: true},
                {Name: "Emotes", Value: emoteText, Inline: false},
            },
        }

        if guild.Icon != "" {
            serverinfoEmbed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: fmt.Sprintf("https://cdn.discordapp.com/icons/%s/%s.jpg", guild.ID, guild.Icon) }
            serverinfoEmbed.URL = fmt.Sprintf("https://cdn.discordapp.com/icons/%s/%s.jpg", guild.ID, guild.Icon)
        }

        _, err = session.ChannelMessageSendEmbed(msg.ChannelID, serverinfoEmbed)
        helpers.Relax(err)
    case "userinfo":
        session.ChannelTyping(msg.ChannelID)
        targetUser, err := session.User(msg.Author.ID)
        helpers.Relax(err)
        args := strings.Split(content, " ")
        if len(args) >= 1 && args[0] != "" {
            targetUser, err = helpers.GetUserFromMention(args[0])
            helpers.Relax(err)
            if targetUser.ID == "" {
                _, err := session.ChannelMessageSend(msg.ChannelID, helpers.GetTextF("bot.arguments.invalid"))
                helpers.Relax(err)
            }
        }

        currentChannel, err := session.Channel(msg.ChannelID)
        helpers.Relax(err)
        currentGuild, err := session.Guild(currentChannel.GuildID)
        helpers.Relax(err)
        targetMember, err := session.GuildMember(currentGuild.ID, targetUser.ID)
        helpers.Relax(err)

        status := ""
        game := ""
        gameUrl := ""
        for _, presence := range currentGuild.Presences {
            if presence.User.ID == targetUser.ID {
                status = string(presence.Status)
                switch status {
                case "dnd":
                    status = "Do Not Disturb"
                case "idle":
                    status = "Away"
                }
                if presence.Game != nil {
                    game = presence.Game.Name
                    gameUrl = presence.Game.URL
                }
            }
        }
        nick := ""
        if targetMember.Nick != "" {
            nick = targetMember.Nick
        }
        description := ""
        if status != "" {
            description = fmt.Sprintf("**%s**", status)
            if game != "" {
                description = fmt.Sprintf("**%s** (Playing: **%s**)", status, game)
                if gameUrl != "" {
                    description = fmt.Sprintf("**%s** (:mega: Streaming: **%s**)", status, game)
                }
            }
        }
        title := fmt.Sprintf("%s#%s", targetUser.Username, targetUser.Discriminator)
        if nick != "" {
            title = fmt.Sprintf("%s#%s ~ %s", targetUser.Username, targetUser.Discriminator, nick)
        }
        rolesText := "None"
        guildRoles, err := session.GuildRoles(currentGuild.ID)
        helpers.Relax(err)
        isFirst := true
        slice.Sort(guildRoles, func(i, j int) bool {
            return guildRoles[i].Position > guildRoles[j].Position
        })
        for _, guildRole := range guildRoles {
            for _, userRole := range targetMember.Roles {
                if guildRole.ID == userRole {
                    if isFirst == true {
                        rolesText = fmt.Sprintf("%s", guildRole.Name)
                    } else {

                        rolesText += fmt.Sprintf(", %s", guildRole.Name)
                    }
                    isFirst = false
                }
            }
        }

        joinedTime := helpers.GetTimeFromSnowflake(targetUser.ID)
        joinedServerTime, err := discordgo.Timestamp(targetMember.JoinedAt).Parse()
        helpers.Relax(err)

        lastAfterMemberId := ""
        var allMembers []*discordgo.Member
        for {
            members, err := session.GuildMembers(currentGuild.ID, lastAfterMemberId, 1000)
            if len(members) <= 0 {
                break
            }
            lastAfterMemberId = members[len(members)-1].User.ID
            helpers.Relax(err)
            for _, u := range members {
                allMembers = append(allMembers, u)
            }
        }
        slice.Sort(allMembers[:], func(i, j int) bool {
            iMemberTime, err := discordgo.Timestamp(allMembers[i].JoinedAt).Parse()
            helpers.Relax(err)
            jMemberTime, err := discordgo.Timestamp(allMembers[j].JoinedAt).Parse()
            helpers.Relax(err)
            return iMemberTime.Before(jMemberTime)
        })
        userNumber := -1
        for i, sortedMember := range allMembers[:] {
            if sortedMember.User.ID == targetUser.ID {
                userNumber = i + 1
                break
            }
        }

        userinfoEmbed := &discordgo.MessageEmbed{
            Color:  0x0FADED,
            Title:  title,
            Footer: &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Member #%d | User ID: %s", userNumber, targetUser.ID)},
            Fields: []*discordgo.MessageEmbedField{
                {Name: "Joined Discord on", Value: fmt.Sprintf("%s (%s)", joinedTime.Format(time.ANSIC), helpers.SinceInDaysText(joinedTime)), Inline: true},
                {Name: "Joined this server on", Value: fmt.Sprintf("%s (%s)", joinedServerTime.Format(time.ANSIC), helpers.SinceInDaysText(joinedServerTime)), Inline: true},
                {Name: "Roles", Value: rolesText, Inline: false},
                {Name: "Voice Stats",
                    Value: fmt.Sprintf("use `%svoicestats @%s` to view the voice stats for this user",
                        helpers.GetPrefixForServer(currentGuild.ID),
                        fmt.Sprintf("%s#%s", targetUser.Username, targetUser.Discriminator)), Inline: false},
            },
        }
        if description != "" {
            userinfoEmbed.Description = description
        }

        if helpers.GetAvatarUrl(targetUser) != "" {
            userinfoEmbed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: helpers.GetAvatarUrl(targetUser)}
            userinfoEmbed.URL = helpers.GetAvatarUrl(targetUser)
        }
        if gameUrl != "" {
            userinfoEmbed.URL = gameUrl
        }

        _, err = session.ChannelMessageSendEmbed(msg.ChannelID, userinfoEmbed)
        helpers.Relax(err)
    case "voicestats":
        session.ChannelTyping(msg.ChannelID)
        targetUser, err := session.User(msg.Author.ID)
        helpers.Relax(err)
        args := strings.Split(content, " ")
        if len(args) >= 1 && args[0] != "" {
            targetUser, err = helpers.GetUserFromMention(args[0])
            helpers.Relax(err)
            if targetUser.ID == "" {
                _, err := session.ChannelMessageSend(msg.ChannelID, helpers.GetTextF("bot.arguments.invalid"))
                helpers.Relax(err)
            }
        }

        channel, err := session.Channel(msg.ChannelID)
        helpers.Relax(err)
        currentConnectionText := "currently not connected to any Voice Channel on this server"
        for _, voiceStateWithTime := range voiceStatesWithTime {
            if voiceStateWithTime.VoiceState.GuildID == channel.GuildID && voiceStateWithTime.VoiceState.UserID == targetUser.ID {
                //duration := time.Since(voiceStateWithTime.JoinTimeUtc)
                currentVoiceChannel, err := session.Channel(voiceStateWithTime.VoiceState.ChannelID)
                helpers.Relax(err)
                currentConnectionText = fmt.Sprintf("Connected to **<#%s>** since **%s**",
                    currentVoiceChannel.ID,
                    helpers.HumanizedTimesSinceText(voiceStateWithTime.JoinTimeUtc))
            }
        }

        title := fmt.Sprintf("Voice Stats for %s", targetUser.Username)

        var entryBucket []DB_VoiceTime
        listCursor, err := rethink.Table("stats_voicetimes").Filter(
            rethink.Row.Field("guildid").Eq(channel.GuildID),
        ).Filter(
            rethink.Row.Field("userid").Eq(targetUser.ID),
        ).Run(helpers.GetDB())
        defer listCursor.Close()
        err = listCursor.All(&entryBucket)

        voiceChannelsDuration := map[string]time.Duration{}

        if err != rethink.ErrEmptyResult && len(entryBucket) > 0 {
            for _, voiceTime := range entryBucket {
                voiceChannelDuration := voiceTime.LeaveTimeUtc.Sub(voiceTime.JoinTimeUtc)
                if _, ok := voiceChannelsDuration[voiceTime.ChannelID]; ok {
                    voiceChannelsDuration[voiceTime.ChannelID] += voiceChannelDuration
                } else {
                    voiceChannelsDuration[voiceTime.ChannelID] = voiceChannelDuration
                }
            }
        } else if err != nil && err != rethink.ErrEmptyResult {
            helpers.Relax(err)
        }

        voicestatsEmbed := &discordgo.MessageEmbed{
            Color:       0x0FADED,
            Title:       title,
            Description: currentConnectionText,
            Footer:      &discordgo.MessageEmbedFooter{Text: "Total times exclude the currently active session"},
            Fields:      []*discordgo.MessageEmbedField{},
        }

        guildChannels, err := session.GuildChannels(channel.GuildID)
        helpers.Relax(err)

        slice.Sort(guildChannels, func(i, j int) bool {
            return guildChannels[i].Position > guildChannels[j].Position
        })

        for _, guildChannel := range guildChannels {
            for voiceChannelID, voiceChannelDuration := range voiceChannelsDuration {
                if voiceChannelID == guildChannel.ID {
                    voiceChannel, err := session.Channel(voiceChannelID)
                    helpers.Relax(err)
                    voicestatsEmbed.Fields = append(voicestatsEmbed.Fields, &discordgo.MessageEmbedField{
                        Name:   fmt.Sprintf("Total time connected for #%s", voiceChannel.Name),
                        Value:  fmt.Sprintf("%s", helpers.HumanizedTimesSinceText(time.Now().UTC().Add(voiceChannelDuration))),
                        Inline: false,
                    })
                }
            }
        }

        _, err = session.ChannelMessageSendEmbed(msg.ChannelID, voicestatsEmbed)
        helpers.Relax(err)
    }
}

func (r *Stats) setVoiceTimeEntry(entry DB_VoiceTime) {
    _, err := rethink.Table("stats_voicetimes").Update(entry).Run(helpers.GetDB())
    helpers.Relax(err)
}

func (r *Stats) getVoiceTimeEntryByOrCreateEmpty(key string, id string) DB_VoiceTime {
    var entryBucket DB_VoiceTime
    listCursor, err := rethink.Table("stats_voicetimes").Filter(
        rethink.Row.Field(key).Eq(id),
    ).Run(helpers.GetDB())
    defer listCursor.Close()
    err = listCursor.One(&entryBucket)

    // If user has no DB entries create an empty document
    if err == rethink.ErrEmptyResult {
        insert := rethink.Table("stats_voicetimes").Insert(DB_VoiceTime{})
        res, e := insert.RunWrite(helpers.GetDB())
        // If the creation was successful read the document
        if e != nil {
            panic(e)
        } else {
            return r.getVoiceTimeEntryByOrCreateEmpty("id", res.GeneratedKeys[0])
        }
    } else if err != nil {
        panic(err)
    }

    return entryBucket
}