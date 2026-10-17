CREATE TABLE IF NOT EXISTS courses (
    code TEXT PRIMARY KEY NOT NULL, -- CSC148H1
    designator TEXT NOT NULL, -- CSC: code[0:3]
    number TEXT NOT NULL, -- 148: code[3:6]
    level INTEGER NOT NULL, -- 100-400: code[3] is 1-4 or A-D
    weight FLOAT NOT NULL, -- 0.5 if code[6] is H, 1.0 if Y
    campus TEXT NOT NULL, -- code[7]: 0 online, 1 utsg, 3 utsc, 5 utm
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    prerequisites TEXT, -- requisite JSON, or NULL if none
    corequisites TEXT, -- requisite JSON, or NULL if none
    exclusions TEXT, -- exclusions JSON, or NULL if none
    breadth_req TEXT -- breadth JSON, or NULL if none
) WITHOUT ROWID;

CREATE INDEX IF NOT EXISTS idx_course_designator ON courses(designator);
CREATE INDEX IF NOT EXISTS idx_course_level ON courses(level);
CREATE INDEX IF NOT EXISTS idx_course_weight ON courses(weight);
CREATE INDEX IF NOT EXISTS idx_course_campus ON courses(campus);