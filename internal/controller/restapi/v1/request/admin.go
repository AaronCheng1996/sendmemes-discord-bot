package request

type AlbumCreate struct {
	Name           string `json:"name" validate:"required"`
	SendMode       string `json:"send_mode"`
	SendConfigJSON string `json:"send_config_json"`
}

type AlbumUpdate struct {
	Name           string `json:"name" validate:"required"`
	SendMode       string `json:"send_mode"`
	SendConfigJSON string `json:"send_config_json"`
}

type ImageCreate struct {
	URL     string `json:"url" validate:"required"`
	Source  string `json:"source"`
	GuildID string `json:"guild_id"`
	AlbumID int    `json:"album_id"`
	FileID  int64  `json:"file_id"`
}

type ImageUpdate struct {
	URL     string `json:"url" validate:"required"`
	Source  string `json:"source"`
	GuildID string `json:"guild_id"`
	AlbumID int    `json:"album_id"`
	FileID  int64  `json:"file_id"`
}

type SchedulePut struct {
	GuildID         string `json:"guild_id"`
	SendChannelID   string `json:"send_channel_id"`
	SendInterval    string `json:"send_interval"`
	SendHistorySize int    `json:"send_history_size"`
}

type ScheduleTriggerNow struct {
	GuildID string `json:"guild_id"`
}

// AlbumSendTest triggers a one-off preview send for the album in the URL path.
type AlbumSendTest struct {
	GuildID string `json:"guild_id"`
}
