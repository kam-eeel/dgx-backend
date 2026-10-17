CREATE TABLE IF NOT EXISTS programs (
    code TEXT PRIMARY KEY NOT NULL, -- ASMIN1001
    type TEXT NOT NULL, -- Specialist, Major, Minor, Focus, or Certificate
    number TEXT NOT NULL, -- 1001
    title TEXT NOT NULL,
    enrolment_reqs TEXT, -- enrolment JSON, or NULL if none
    completion_reqs TEXT, -- completion JSON, or NULL if none
    program_area TEXT -- areas JSON, or NULL if none
) WITHOUT ROWID;

CREATE INDEX IF NOT EXISTS idx_program_type ON programs(type);