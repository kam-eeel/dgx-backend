CREATE TABLE IF NOT EXISTS programs (
    code TEXT PRIMARY KEY NOT NULL,
    type TEXT NOT NULL,
    number TEXT NOT NULL,
    title TEXT NOT NULL,
    enrolment_reqs TEXT,
    completion_reqs TEXT,
    program_area TEXT -- json string array
) WITHOUT ROWID;

CREATE INDEX IF NOT EXISTS idx_program_type ON programs(type);