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
// album behind a post into a thread. Every album delivery carries it except
// Video mode, whose albums are the ones deliberately too large to post in full.
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

// fullAlbumMorePrefix marks the "post the next page" button that closes a paged
// full-album post. The album id and the offset to resume from follow, colon
// separated. It deliberately does not start with fullAlbumButtonPrefix so the
// two buttons cannot be confused for one another.
const fullAlbumMorePrefix = "fullalbum_more:"

// fullAlbumMoreCustomID builds the CustomID for the continue button of a paged
// full-album post, resuming at offset (an index into the album's non-cover images).
func fullAlbumMoreCustomID(albumID, offset int) string {
	return fmt.Sprintf("%s%d:%d", fullAlbumMorePrefix, albumID, offset)
}

// parseFullAlbumMoreCustomID extracts the album id and resume offset from a
// continue-button CustomID. ok is false when the CustomID belongs to another
// button or either value is not a non-negative integer.
func parseFullAlbumMoreCustomID(customID string) (albumID, offset int, ok bool) {
	rest, found := strings.CutPrefix(customID, fullAlbumMorePrefix)
	if !found {
		return 0, 0, false
	}
	idPart, offsetPart, found := strings.Cut(rest, ":")
	if !found {
		return 0, 0, false
	}
	id, err := strconv.Atoi(idPart)
	if err != nil {
		return 0, 0, false
	}
	off, err := strconv.Atoi(offsetPart)
	if err != nil || off < 0 {
		return 0, 0, false
	}
	return id, off, true
}

// fullAlbumMoreButtonRow returns the action row closing a page of a large album,
// labelled with how many images are still waiting.
func fullAlbumMoreButtonRow(albumID, offset, remaining int) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    fmt.Sprintf("📥 Post %d more", remaining),
					Style:    discordgo.PrimaryButton,
					CustomID: fullAlbumMoreCustomID(albumID, offset),
				},
			},
		},
	}
}
