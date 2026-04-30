package repo

// AlbumAdminListQuery drives filtered/sorted admin album listing.
// Zero value: sort by id ascending, no filter (matches legacy behaviour).
type AlbumAdminListQuery struct {
	SortBy    string // id | name | positive_rating | cover
	SortAsc   bool
	FilterCol string // empty = none | all | id | name | positive_rating | cover
	FilterQ   string
}

// ImageAdminListQuery drives filtered/sorted admin image listing.
// AlbumScopeID > 0 restricts to that album (same as historical album_id query param).
type ImageAdminListQuery struct {
	AlbumScopeID int
	SortBy       string // id | album_id | url | source | guild_id | file_id
	SortAsc      bool
	FilterCol    string // empty = none | all | id | album_id | url | source | guild_id | file_id
	FilterQ      string
}
