-- Name the course breadth and program areas columns after their JSON fields
-- (breadth, areas). The stored values are unchanged.
--
-- The renamed keys inside prerequisites, corequisites and exclusions (min_gpa,
-- max_amount, courses, the text kinds membership and ambiguity) and the
-- admission_category values are rewritten by the next scrape.
ALTER TABLE courses RENAME COLUMN breadth_req TO breadth;
ALTER TABLE programs RENAME COLUMN program_area TO areas;
