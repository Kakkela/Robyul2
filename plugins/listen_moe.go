package plugins

import (
    "time"
    "sync"
    "github.com/sn0w/discordgo"
    "git.lukas.moe/sn0w/Karen/logger"
    "git.lukas.moe/sn0w/Karen/helpers"
    "strings"
    "os/exec"
    "bufio"
    "encoding/binary"
    "io"
    "git.lukas.moe/sn0w/radio-b"
    "git.lukas.moe/sn0w/Karen/cache"
    "github.com/gorilla/websocket"
    "net/url"
)

// ---------------------------------------------------------------------------------------------------------------------
// Helper structs for managing and closing voice connections
// ---------------------------------------------------------------------------------------------------------------------
var RadioChan *radio.Radio

var RadioCurrentMeta RadioMeta

type RadioMetaContainer struct {
    SongId      float64 `json:"song_id,omitempty"`
    ArtistName  string `json:"artist_name"`
    SongName    string `json:"song_name"`
    AnimeName   string `json:"anime_name,omitempty"`
    RequestedBy string `json:"requested_by,omitempty"`
    Listeners   float64 `json:"listeners,omitempty"`
}

type RadioMeta struct {
    RadioMetaContainer

    Last       RadioMetaContainer `json:"last,omitempty"`
    SecondLast RadioMetaContainer `json:"second_last,omitempty"`
}

type RadioGuildConnection struct {
    sync.RWMutex

    // Closer channel for stop commands
    closer    chan struct{}

    // Marks this guild as streaming music
    streaming bool
}

func (r *RadioGuildConnection) Alloc() *RadioGuildConnection {
    r.Lock()
    r.streaming = false
    r.closer = make(chan struct{})
    r.Unlock()

    return r
}

func (r *RadioGuildConnection) Close() {
    r.Lock()
    close(r.closer)
    r.streaming = false
    r.Unlock()
}

// ---------------------------------------------------------------------------------------------------------------------
// Actual plugin implementation
// ---------------------------------------------------------------------------------------------------------------------
type ListenDotMoe struct {
    connections map[string]*RadioGuildConnection
}

func (l *ListenDotMoe) Commands() []string {
    return []string{
        "moe",
        "lm",
    }
}

func (l *ListenDotMoe) Init(session *discordgo.Session) {
    l.connections = make(map[string]*RadioGuildConnection)

    go l.streamer()
    go l.tracklistWorker()
}

func (l *ListenDotMoe) Action(command string, content string, msg *discordgo.Message, session *discordgo.Session) {
    // Sanitize subcommand
    content = strings.TrimSpace(content)

    // Store channel ref
    channel, err := cache.Channel(msg.ChannelID)
    helpers.Relax(err)

    // Only continue if the voice is available
    if !helpers.VoiceIsFreeOrOccupiedBy(channel.GuildID, "listen.moe") {
        helpers.VoiceSendStatus(channel.ID, channel.GuildID, session)
        return
    }

    // Store guild ref
    guild, err := session.Guild(channel.GuildID)
    helpers.Relax(err)

    // Store voice channel ref
    vc := l.resolveVoiceChannel(msg.Author, guild, session)

    // Store voice connection ref (deferred)
    var voiceConnection *discordgo.VoiceConnection

    // Check if the user is connected to voice at all
    if vc == nil {
        session.ChannelMessageSend(channel.ID, "You're either not in the voice-chat or I can't see you :neutral_face:")
        return
    }

    // He is connected for sure.
    // The routine would've stopped otherwise
    // Check if we are present in this channel too
    if session.VoiceConnections[guild.ID] == nil || session.VoiceConnections[guild.ID].ChannelID != vc.ID {
        // Nope.
        // Check if the user wanted us to join.
        // Else report the error
        if content == "join" {
            helpers.RequireAdmin(msg, func() {
                helpers.VoiceOccupy(guild.ID, "listen.moe")

                message, merr := session.ChannelMessageSend(channel.ID, ":arrows_counterclockwise: Joining...")

                voiceConnection, err = session.ChannelVoiceJoin(guild.ID, vc.ID, false, false)
                helpers.Relax(err)

                if merr == nil {
                    session.ChannelMessageEdit(channel.ID, message.ID, "Joined!\nThe radio should start playing shortly c:")

                    l.connections[guild.ID] = (&RadioGuildConnection{}).Alloc()

                    go l.pipeStream(guild.ID, session)
                    return
                }

                helpers.Relax(merr)
            })
        } else {
            session.ChannelMessageSend(channel.ID, "You should join the channel I'm in or make me join yours before telling me to do stuff :thinking:")
        }

        return
    }

    // We are present.
    // Check for other commands
    switch content {
    case "leave", "l":
        helpers.RequireAdmin(msg, func() {
            voiceConnection.Disconnect()

            l.connections[guild.ID].Lock()
            l.connections[guild.ID].Close()
            delete(l.connections, guild.ID)

            session.ChannelMessageSend(channel.ID, "OK, bye :frowning:")
        })
        break

    case "playing", "np", "song", "title":
        fields := make([]*discordgo.MessageEmbedField, 1)
        fields[0] = &discordgo.MessageEmbedField{
            Name: "Now Playing",
            Value: RadioCurrentMeta.ArtistName + " " + RadioCurrentMeta.SongName,
            Inline: false,
        }

        if RadioCurrentMeta.AnimeName != "" {
            fields = append(fields, &discordgo.MessageEmbedField{
                Name: "Anime", Value: RadioCurrentMeta.AnimeName, Inline: false,
            })
        }

        if RadioCurrentMeta.RequestedBy != "" {
            fields = append(fields, &discordgo.MessageEmbedField{
                Name: "Requested by", Value: "[" + RadioCurrentMeta.RequestedBy + "](https://forum.listen.moe/u/" + RadioCurrentMeta.RequestedBy + ")", Inline: false,
            })
        }

        session.ChannelMessageSendEmbed(msg.ChannelID, &discordgo.MessageEmbed{
            Color: 0xEC1A55,
            Thumbnail: &discordgo.MessageEmbedThumbnail{
                URL: "http://i.imgur.com/Jp8N7YG.jpg",
            },
            Fields: fields,
            Footer: &discordgo.MessageEmbedFooter{
                Text: "powered by listen.moe (ﾉ◕ヮ◕)ﾉ*:･ﾟ✧",
            },
        })
        break
    }
}

// ---------------------------------------------------------------------------------------------------------------------
// Helper functions for managing voice connections
// ---------------------------------------------------------------------------------------------------------------------

// Resolves a voice channel relative to a user id
func (l *ListenDotMoe) resolveVoiceChannel(user *discordgo.User, guild *discordgo.Guild, session *discordgo.Session) *discordgo.Channel {
    for _, vs := range guild.VoiceStates {
        if vs.UserID == user.ID {
            channel, err := session.Channel(vs.ChannelID)
            if err != nil {
                return nil
            }

            return channel
        }
    }

    return nil
}

// ---------------------------------------------------------------------------------------------------------------------
// Helper functions for reading and piping listen.moe's stream to multiple targets at once
// ---------------------------------------------------------------------------------------------------------------------

func (l *ListenDotMoe) streamer() {
    logger.PLUGIN.L("listen_moe", "Allocating channels")
    RadioChan = radio.NewRadio()

    logger.PLUGIN.L("listen_moe", "Piping subprocesses")

    // Read stream with ffmpeg and turn it into PCM
    ffmpeg := exec.Command(
        "ffmpeg",
        "-i", "http://listen.moe:9999/stream",
        "-f", "s16le",
        "pipe:1",
    )
    ffout, err := ffmpeg.StdoutPipe()
    helpers.Relax(err)

    // Pipe FFMPEG to ropus to convert it to .ro format
    ropus := exec.Command("ropus")
    ropus.Stdin = ffout

    rout, err := ropus.StdoutPipe()
    helpers.Relax(err)

    logger.PLUGIN.L("listen_moe", "Running FFMPEG")

    // Run ffmpeg
    err = ffmpeg.Start()
    helpers.Relax(err)

    logger.PLUGIN.L("listen_moe", "Running ROPUS")

    // Run ropus
    err = ropus.Start()
    helpers.Relax(err)

    // Stream ropus to buffer
    robuf := bufio.NewReaderSize(rout, 16384)

    // Stream ropus output to discord
    var opusLength int16

    logger.PLUGIN.L("listen_moe", "Streaming :3")
    for {
        // Read opus frame length
        err = binary.Read(robuf, binary.LittleEndian, &opusLength)
        if err == io.EOF || err == io.ErrUnexpectedEOF {
            break
        }
        helpers.Relax(err)

        // Read audio data
        opus := make([]byte, opusLength)
        err = binary.Read(robuf, binary.LittleEndian, &opus)
        if err == io.EOF || err == io.ErrUnexpectedEOF {
            break
        }
        helpers.Relax(err)

        // Send to discord
        RadioChan.Broadcast(opus)
    }

    logger.PLUGIN.L("listen_moe", "Stream died")
}

func (l *ListenDotMoe) pipeStream(guildID string, session *discordgo.Session) {
    audioChan, id := RadioChan.Listen()
    vc := session.VoiceConnections[guildID]

    vc.Speaking(true)

    // Start eventloop
    for {
        // Exit if the closer channel dies
        select {
        case <-l.connections[guildID].closer:
            return
        default:
        }

        // Do nothing until voice is ready
        if !vc.Ready {
            time.Sleep(1 * time.Second)
            continue
        }

        // Send a frame to discord
        vc.OpusSend <- (<-audioChan)
    }

    vc.Speaking(false)

    RadioChan.Stop(id)
}

// ---------------------------------------------------------------------------------------------------------------------
// Helper functions for interacting with listen.moe's api
// ---------------------------------------------------------------------------------------------------------------------
func (l *ListenDotMoe) tracklistWorker() {
    for {
        c, _, err := websocket.DefaultDialer.Dial((&url.URL{
            Scheme: "wss",
            Host: "listen.moe",
            Path: "/api/v2/socket",
        }).String(), nil)

        helpers.Relax(err)

        c.WriteJSON(map[string]string{"token":helpers.GetConfig().Path("listen_moe").Data().(string)})
        helpers.Relax(err)

        for {
            time.Sleep(5 * time.Second)
            err := c.ReadJSON(&RadioCurrentMeta)

            if err == io.ErrUnexpectedEOF {
                continue
            }

            if err != nil {
                break
            }
        }

        logger.WARNING.L("listen_moe", "Connection to wss://listen.moe lost. Reconnecting!")
        c.Close()

        time.Sleep(5 * time.Second)
    }
}
