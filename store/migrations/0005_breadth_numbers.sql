-- Store a course's breadth categories as their numbers (1-5), as credits and
-- average nodes do: ["Thought, Belief and Behaviour (2)"] -> [2]. Every name
-- ends in "(n)".
UPDATE courses
SET breadth = (SELECT json_group_array(CAST(substr(j.value, -2, 1) AS INTEGER)) FROM json_each(courses.breadth) AS j)
WHERE breadth IS NOT NULL;
