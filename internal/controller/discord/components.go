package discord

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// fullAlbumButtonPrefix is the CustomID prefix carried by the "Full album"
// button attached to random scheduled posts. The album id follows the colon.
const fullAlbumButtonPrefix = "fullalbum:"

// fullAlbumCustomID builds the CustomID for a Full-album button targeting albumID.
func fullAlbumCustomID(albumID int) string {
	return fmt.Sprintf("%s%d", fullAlbumButtonPrefix, albumID)
}

// parseFullAlbumCustomID extracts the album id from a Full-album button CustomID.
// ok is false when the CustomID does not belong to that button or the trailing
// value is not a valid integer.
func parseFullAlbumCustomID(customID string) (albumID int, ok bool) {
	rest, found := strings.CutPrefix(customID, fullAlbumButtonPrefix)
	if !found {
		return 0, false
	}
	id, err := strconv.Atoi(rest)
	if err != nil {
		return 0, false
	}
	return id, true
}

// fullAlbumButtonRow returns a one-button action row that lets anyone expand the
// album behind a random post into a thread.
func fullAlbumButtonRow(albumID int) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "📖 Full album",
					Style:    discordgo.SecondaryButton,
					CustomID: fullAlbumCustomID(albumID),
				},
			},
		},
	}
}
