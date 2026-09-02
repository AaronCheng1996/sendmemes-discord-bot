package repo

// AlbumAdminListQuery drives filtered/sorted admin album listing.
// Zero value: sort by id ascending, no filter, missing albums hidden.
type AlbumAdminListQuery struct {
	SortBy    string // id | name | positive_rating | cover
	SortAsc   bool
	FilterCol string // empty = none | all | id | name | positive_rating | cover
	FilterQ   string
	// IncludeMissing brings back albums whose source folder disappeared. They
	// are hidden by default so a reorganized media library does not leave the
	// dashboard full of rows nothing can be sent from.
	IncludeMissing bool
}

// TaskRunListQuery narrows a task-run listing. The zero value returns every
// run, newest first.
type TaskRunListQuery struct {
	Source string // exact match, e.g. "scheduled_send"; empty = any
	Status string // running | succeeded | failed; empty = any
}

// ImageAdminListQuery drives filtered/sorted admin image listing.
// AlbumScopeID > 0 restricts to that album (same as historical album_id query param).
// Zero value hides soft-deleted rows.
type ImageAdminListQuery struct {
	AlbumScopeID int
	SortBy       string // id | album_id | url | source | guild_id | file_id
	SortAsc      bool
	FilterCol    string // empty = none | all | id | album_id | url | source | guild_id | file_id
	FilterQ      string
	// IncludeDeleted brings back rows a sync soft-deleted because the source no
	// longer lists the file. Hidden by default, like missing albums.
	IncludeDeleted bool
}
