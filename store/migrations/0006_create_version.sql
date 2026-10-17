-- One row per school year the calendar was scraped for.
CREATE TABLE IF NOT EXISTS version (
    school_year TEXT PRIMARY KEY NOT NULL, -- "2026-2027"
    last_updated TEXT NOT NULL -- RFC 3339, UTC
) WITHOUT ROWID;
