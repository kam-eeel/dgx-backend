-- Name the requirement columns after the Program JSON fields (enrolment,
-- completion). The stored values are unchanged; the enrolment JSON key
-- "exclude" -> "exclusions" and the profile attribute renames are rewritten
-- by the next scrape.
ALTER TABLE programs RENAME COLUMN enrolment_reqs TO enrolment;
ALTER TABLE programs RENAME COLUMN completion_reqs TO completion;
