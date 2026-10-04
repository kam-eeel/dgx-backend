CREATE TABLE IF NOT EXISTS courses (
    code TEXT PRIMARY KEY NOT NULL,
    designator TEXT NOT NULL,
    number TEXT NOT NULL,
    level INTEGER NOT NULL,
    weight FLOAT NOT NULL,
    campus INTEGER NOT NULL,
    title TEXT NOT NULL,
    prerequisites TEXT, -- json string array
    corequisites TEXT, -- json string array
    exclusions TEXT, -- json string array
    breadth_req TEXT, -- json string array
    distribution_req TEXT, -- json string array
) WITHOUT ROWID;

CREATE INDEX IF NOT EXISTS idx_course_designator ON courses(course_designator);
CREATE INDEX IF NOT EXISTS idx_course_level ON courses(course_level);
CREATE INDEX IF NOT EXISTS idx_course_weight ON courses(course_weight);
CREATE INDEX IF NOT EXISTS idx_course_campus ON courses(course_campus);