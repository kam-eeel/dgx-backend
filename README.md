# dgx-backend

dgx-backend is the backend for a student project in [CSC207H1](https://artsci.calendar.utoronto.ca/course/csc207h1): a version of the University of Toronto's Degree Explorer. It scrapes every course and program in the Faculty of Arts & Science academic calendar at `artsci.calendar.utoronto.ca`. It parses course prerequisites, corequisites, and exclusions, and program enrolment and completion requirements, into JSON requirement trees stored in SQLite. At startup and daily at 5 AM `TZ` time (default `America/Toronto`, UTC−5 or UTC−4), it scrapes again if no data exists for the current school year. An HTTP API serves single courses and programs, and the whole database as a file.

## Contents

- [Endpoints](#endpoints): [health](#health), [database file](#database-file), [course](#course), [program](#program)
- [Schema](#schema): [conventions](#conventions), [requirement nodes](#requirement-nodes)
  - [Courses table](#courses-table): [prerequisites and corequisites](#prerequisites-and-corequisites), [exclusions](#exclusions), [breadth](#breadth)
  - [Programs table](#programs-table): [enrolment](#enrolment), [completion](#completion), [areas](#areas)
  - [Version table](#version-table)

## Endpoints

The server listens on port `8080` and accepts only `GET` requests. Every response except `/db` is JSON. The server returns these errors:

| Status | When | Body |
|---|---|---|
| `404 Not Found` | A course or program lookup fails. | `{"error": "get course XYZ999H1: not found"}` |
| `429 Too Many Requests` | A client exceeds 5 requests per second, in bursts of up to 5. IPv6 clients share one limit per /64 prefix. The response has `Retry-After: 1`. | `{"error": "rate limit exceeded"}` |
| `500 Internal Server Error` | A course or program lookup fails for another reason. The server logs the cause. | `{"error": "internal server error"}` |

The server reads the client IP address from the `CF-Connecting-IP` header, or from the connection if the header is missing.

Every response has `X-Content-Type-Options: nosniff`. A successful course, program, or `/db` response has `Cache-Control: public, max-age=3600`, and every other response has `Cache-Control: no-store`.

### Health

```sh
curl http://localhost:8080/health
```

```json
{
  "status": "healthy",
  "time": "2026-10-17T23:18:01.544921Z"
}
```

`time` is in UTC. `status` is one of the following values:

| `status` | When |
|---|---|
| `starting up` | From process start until the server schedules the update job, including writing the `/db` snapshot if it's missing. |
| `checking for updates` | The update job is reading the school year from the `version` table. |
| `updating database` | The update job is scraping the calendar, including any waits while the calendar site rate-limits the job. |
| `healthy` | Any other time. |

### Database file

`GET /db` downloads `dgx.db`, a SQLite copy of the database with the same [schema](#schema). The server writes the copy at startup if it's missing, and after each successful update. The endpoint supports range requests and `If-Modified-Since`.

```sh
curl -OJ http://localhost:8080/db
```

```http
HTTP/1.1 200 OK
Accept-Ranges: bytes
Cache-Control: public, max-age=3600
Content-Disposition: attachment; filename="dgx.db"
Content-Length: 12275712
Content-Type: application/octet-stream
Last-Modified: Sat, 17 Oct 2026 23:17:54 GMT
X-Content-Type-Options: nosniff
```

### Course

`GET /course/{id}` returns the [courses table](#courses-table) row whose `code` is `{id}`, matched exactly and case-sensitively. JSON columns are JSON values, and SQL `NULL` is `null`.

```sh
curl http://localhost:8080/course/PHY152H1
```

<details>
<summary>Response</summary>

```json
{
  "code": "PHY152H1",
  "designator": "PHY",
  "number": "152",
  "level": 100,
  "weight": 0.5,
  "campus": "utsg",
  "title": "Foundations of Physics II",
  "description": "The concept of fields will be introduced and discussed in the context of gravity and electricity. Topics include rotational motion, oscillations, waves, electricity and magnetism.",
  "prerequisites": {
    "req": {
      "type": "or",
      "items": [
        {
          "type": "course",
          "code": "PHY131H1"
        },
        {
          "type": "course",
          "code": "PHY151H1"
        }
      ]
    },
    "raw": "PHY131H1/  PHY151H1"
  },
  "corequisites": {
    "req": {
      "type": "or",
      "items": [
        {
          "type": "course",
          "code": "MAT136H1"
        },
        {
          "type": "course",
          "code": "MAT149H1"
        },
        {
          "type": "course",
          "code": "MAT137Y1"
        },
        {
          "type": "course",
          "code": "MAT159H1"
        },
        {
          "type": "course",
          "code": "MAT157Y1"
        }
      ]
    },
    "notes": [
      "MAT149H1 recommended"
    ],
    "raw": "MAT136H1/  MAT149H1/  MAT137Y1/  MAT159H1/  MAT157Y1 ( MAT149H1 recommended)"
  },
  "exclusions": {
    "items": [
      {
        "courses": [
          "PHY132H1"
        ]
      }
    ],
    "raw": "PHY132H1"
  },
  "breadth": [
    5
  ]
}
```

</details>

### Program

`GET /program/{id}` returns the [programs table](#programs-table) row whose `code` is `{id}`, in the same form as a course.

```sh
curl http://localhost:8080/program/ASMIN0667
```

<details>
<summary>Response</summary>

```json
{
  "code": "ASMIN0667",
  "type": "Minor",
  "number": "0667",
  "title": "History and Philosophy of Science and Technology Minor (Arts Program)",
  "enrolment": {
    "kind": "open",
    "req": {
      "type": "credits",
      "amount": 4
    },
    "raw": "This is an open enrolment program. A student who has completed 4.0 credits may enrol in the program."
  },
  "completion": {
    "requirements": [
      {
        "number": 1,
        "kind": "allocation",
        "req": {
          "type": "credits",
          "amount": 4,
          "designators": [
            "HPS"
          ],
          "courses": [
            "JHE353H1",
            "JHE355H1",
            "JPH311H1",
            "PHL245H1",
            "PHL356H1",
            "PHL357H1"
          ]
        },
        "raw": "(4.0 credits)\n3. Additional HPS courses to a total of 4.0 credits, with at least 1.0 credit at the 300+level. JHE353H1, JHE355H1, JPH311H1, PHL245H1, PHL356H1, or PHL357H1 may also be used to fulfill this requirement."
      },
      {
        "number": 2,
        "kind": "limit",
        "parent": 1,
        "req": {
          "type": "credits",
          "designators": [
            "HPS"
          ],
          "levels": [
            100
          ],
          "max_amount": 1
        },
        "raw": "Up to 1.0 credit in HPS courses at the 100-level will be counted towards the minor"
      },
      {
        "number": 3,
        "kind": "constraint",
        "parent": 1,
        "req": {
          "type": "credits",
          "amount": 1,
          "levels": [
            300,
            400
          ]
        },
        "raw": "with at least 1.0 credit at the 300+level"
      }
    ],
    "notes": [
      "Recommended: 0.5 credit from any HPS 100-level courses",
      "Recommended: at least 0.5 credit from any HPS 200-level courses"
    ],
    "raw": "(4.0 credits)\n1. Recommended: 0.5 credit from any HPS 100-level courses. Up to 1.0 credit in HPS courses at the 100-level will be counted towards the minor.\n2. Recommended: at least 0.5 credit from any HPS 200-level courses.\n3. Additional HPS courses to a total of 4.0 credits, with at least 1.0 credit at the 300+level. JHE353H1, JHE355H1, JPH311H1, PHL245H1, PHL356H1, or PHL357H1 may also be used to fulfill this requirement."
  },
  "areas": [
    "History and Philosophy of Science and Technology"
  ]
}
```

</details>

## Schema

The database is SQLite, stored at `data/app.db`. The migrations in `store/migrations` create it and record the last one applied in `PRAGMA user_version`. JSON columns are `TEXT`.

### Conventions

- An optional JSON field is absent when it's empty, `0`, or `false`. The exception is `max_amount`, where `0` means that no matching credits count.
- `raw` is the calendar text that a value comes from. `notes` is calendar text that isn't a requirement, such as advice.
- Course codes match `[A-Z]{3}[A-Z0-9]\d{2}[HY]\d`, for example `CSC148H1`. Program codes match `AS(SPE|MAJ|MIN|FOC|CER)\d{4}[A-Z]?`, for example `ASMAJ1618`.
- Grades are integer percentages. A GPA is on the 4.0 scale.
- A requirement is *Met*, *NotMet*, or *Review*. Review means that a person must decide.
- Types are JSON types. An `integer` is always whole. A `number` is a double, written without a fraction when it's whole, for example `4`. `T[]` is an array of `T`, and `node` is a [requirement node](#requirement-nodes). Each **JSON Schema and Field Types** section ends with a table of every field's type.

### Requirement nodes

Prerequisites, corequisites, enrolment, and completion build their requirements from one node type.

<details>
<summary>JSON Schema and Field Types</summary>

```json
{
  "$id": "urn:dgx:node",
  "type": "object",
  "required": ["type"],
  "additionalProperties": false,
  "properties": {
    "type": {
      "enum": [
        "and", "or", "choose", "not", "course", "credits", "average", "gpa", "program",
        "requirement", "profile", "text"
      ]
    },
    "items": {"type": "array", "items": {"$ref": "urn:dgx:node"}},
    "count": {"type": "integer", "minimum": 1},
    "code": {"type": "string"},
    "min_grade": {"type": "integer", "minimum": 0, "maximum": 100},
    "amount": {"type": "number", "minimum": 0},
    "max_amount": {"type": "number", "minimum": 0},
    "courses": {"type": "array", "items": {"type": "string"}},
    "designators": {"type": "array", "items": {"type": "string"}},
    "levels": {"type": "array", "items": {"enum": [100, 200, 300, 400]}},
    "breadth": {"type": "array", "items": {"enum": [1, 2, 3, 4, 5]}},
    "lists": {"type": "array", "items": {"type": "string"}},
    "requirements": {"type": "array", "items": {"type": "integer", "minimum": 1}},
    "exclude": {"type": "array", "items": {"type": "string"}},
    "exclude_designators": {"type": "array", "items": {"type": "string"}},
    "max_course_amount": {"type": "number", "minimum": 0},
    "transfer": {"type": "boolean"},
    "cr_ncr": {"type": "boolean"},
    "best": {"type": "boolean"},
    "weight": {"type": "number", "minimum": 0},
    "min_gpa": {"type": "number", "minimum": 0, "maximum": 4},
    "scope": {"type": "string"},
    "programs": {"type": "array", "items": {"type": "string"}},
    "attr": {"type": "string"},
    "value": {"type": ["string", "integer", "boolean"]},
    "kind": {
      "enum": [
        "permission", "membership", "placement", "preparation", "admission", "ambiguity", "content",
        "other"
      ]
    },
    "text": {"type": "string"},
    "concurrent": {"type": "boolean"}
  }
}
```

| Field | Type | Required on | Optional on |
|---|---|---|---|
| `type` | `string` | Every node | |
| `items` | `node[]` | `and`, `or`, `choose`, `not` | `average` |
| `count` | `integer` | `choose` | |
| `code` | `string` | `course` | |
| `min_grade` | `integer` | `average` | `course`, `credits` |
| `amount` | `number` | | `credits`, `average` |
| `max_amount` | `number` | | `credits` |
| `courses` | `string[]` | | `credits`, `average` |
| `designators` | `string[]` | | `credits`, `average` |
| `levels` | `integer[]` | | `credits`, `average` |
| `breadth` | `integer[]` | | `credits`, `average` |
| `lists` | `string[]` | | `credits`, `average` |
| `requirements` | `integer[]` | `requirement` | `credits`, `average` |
| `exclude` | `string[]` | | `credits`, `average` |
| `exclude_designators` | `string[]` | | `credits` |
| `max_course_amount` | `number` | | `credits` |
| `transfer` | `boolean` | | `credits` |
| `cr_ncr` | `boolean` | | `credits` |
| `best` | `boolean` | | `average` |
| `weight` | `number` | | `credits`, `and` |
| `min_gpa` | `number` | `gpa` | |
| `scope` | `string` | | `gpa` |
| `programs` | `string[]` | `program` | |
| `attr` | `string` | `profile` | |
| `value` | `string`, `integer`, or `boolean`, by [`attr`](#profile-attributes) | `profile` | |
| `kind` | `string` | `text` | |
| `text` | `string` | `program`, `profile`, `text` | |
| `concurrent` | `boolean` | | Every node |

</details>

| `type` | Met when | Example |
|---|---|---|
| `and` | Every item is met. | `{"type": "and", "items": [{"type": "credits", "amount": 15}, {"type": "gpa", "min_gpa": 3}]}` |
| `or` | Any item is met. | `{"type": "or", "items": [{"type": "course", "code": "DRM101Y1"}, {"type": "credits", "amount": 4}]}` |
| `choose` | At least `count` items are met. | `{"type": "choose", "count": 2, "items": [...]}` |
| `not` | The one item isn't met. | `{"type": "not", "items": [{"type": "course", "code": "CSC207H1"}]}` |
| `course` | The student passed `code`, with at least `min_grade` if set. | `{"type": "course", "code": "CSC148H1", "min_grade": 60}` |
| `credits` | The [matching courses](#course-filters) sum to at least `amount` credits. | `{"type": "credits", "amount": 1, "levels": [300, 400]}` |
| `average` | The credit-weighted average grade of the matching courses is at least `min_grade`. | `{"type": "average", "min_grade": 73, "designators": ["ECO"]}` |
| `gpa` | The GPA is at least `min_gpa`, over `scope` if set. | `{"type": "gpa", "min_gpa": 3, "scope": "CHM credits"}` |
| `program` | The student is enrolled in any of `programs`. | `{"type": "program", "programs": ["ASMAJ0652"], "text": "History Major"}` |
| `requirement` | Any completion row in `requirements` is met. Completion only. | `{"type": "requirement", "requirements": [17]}` |
| `profile` | The student fact `attr` matches `value`. See [profile attributes](#profile-attributes). | `{"type": "profile", "attr": "faculty", "value": "Daniels", "text": "Daniels students"}` |
| `text` | Never. A `text` node is always Review. See [text kinds](#text-kinds). | `{"type": "text", "kind": "permission", "text": "permission of Program Director"}` |

On any node, `concurrent: true` lets a course in progress count as taken within that subtree, for example `{"type": "course", "code": "MAT257Y1", "concurrent": true}`.

#### Course filters

A course matches a `credits` or `average` node if it's in `courses` or has one of `designators`. If neither is set, any course matches. Each other filter that's set narrows the match:

| Field | The course matches if | Example |
|---|---|---|
| `levels` | Its level is in the list. | `"levels": [300, 400]` |
| `breadth` | It has a breadth category in the list. | `"breadth": [4, 5]` |
| `exclude` | It isn't in the list. | `"exclude": ["HIS262H1"]` |
| `exclude_designators` | Its designator isn't in the list. Completion only. | `"exclude_designators": ["FRE", "FSL"]` |
| `lists` | It's on a named list in `completion.lists`. | `"lists": ["Group A", "Group B"]` |
| `requirements` | It's in the pool of a completion row in the list. | `"requirements": [3]` |
| `max_course_amount` | It's worth at most this many credits. Completion only. | `"max_course_amount": 0.5` |
| `min_grade` | Its grade is at least this. | `"min_grade": 80` |
| `transfer` | It's a transfer credit. | `"transfer": true` |
| `cr_ncr` | The student took it Credit/No Credit (CR/NCR). | `"cr_ncr": true` |

#### Amounts

| Node | Field | Meaning | Example |
|---|---|---|---|
| `credits` | `amount` | Credits needed. | `{"type": "credits", "amount": 4}` |
| `credits` | `max_amount` | At most this many matching credits count. | `{"type": "credits", "amount": 4, "max_amount": 8.5}` |
| `credits`, `and` | `weight` | The credit that each matched course, or a pair of courses, contributes. | `{"type": "credits", "courses": ["HIS208Y1"], "weight": 0.5}` |
| `average` | `amount` | The student also needs this many matching credits. | `{"type": "average", "min_grade": 73, "amount": 3}` |
| `average` | `best` | With `amount`, only the highest grades count toward the average. | `"amount": 3, "best": true` |
| `average` | `items` | Replaces the filters: one `credits` node per part, with `amount` as their sum. | `"items": [{"type": "credits", "amount": 0.5, "courses": ["MAT136H1"]}, ...]` |

#### Profile attributes

The `attr` name sets the type of `value` and how to compare it:

| `attr` pattern | Comparison | `attr` values | Example |
|---|---|---|---|
| Plain noun | The fact equals `value`. | `admission_category`, `faculty`, `high_school_course`, `language_background` | `"attr": "faculty", "value": "Daniels"` |
| `has_*` | The fact is true. `value` is `true`. | `has_learning_community`, `has_missing_numeric_grades`, `has_transfer_credits` | `"attr": "has_transfer_credits", "value": true` |
| `min_*`, `max_*` | The fact is at least, or at most, the integer `value`. | `min_year_of_study`, `min_chinese_level`, `min_japanese_level`, `min_korean_level`, `max_attempts_per_course`, `max_attempts_per_requirement`, `max_cr_ncr`, `max_months_from_start` | `"attr": "min_year_of_study", "value": 3` |
| `*_by` | Done on or before the month and day in `value`, written as ISO 8601 `--MM-DD`. | `complete_by` | `"attr": "complete_by", "value": "--05-01"` |

#### Text kinds

| `kind` | Meaning | Example `text` |
|---|---|---|
| `permission` | A person or process decides. | `permission of Program Director` |
| `membership` | Membership in a group that no program code resolves. | `For Arts & Science students` |
| `placement` | A placement test or language proficiency. | `No previous knowledge of French` |
| `preparation` | High school or other prior background. | `Grade 12 Biology or equivalent` |
| `admission` | The department decides at admission time. | `variable minimum grade average` |
| `ambiguity` | The branch that only the lenient reading of two-reading wording accepts. Always inside an `or`. | `a course may count toward both requirements` |
| `content` | Course content decides, which the catalogue doesn't record. Completion only. | `offered by the Canadian Studies Program` |
| `other` | Anything else. | `Will vary depending upon year` |

### Courses table

```sql
CREATE TABLE courses (
    code TEXT PRIMARY KEY NOT NULL,   -- CSC148H1
    designator TEXT NOT NULL,         -- CSC: code[0:3]
    number TEXT NOT NULL,             -- 148: code[3:6]
    level INTEGER NOT NULL,           -- 100-400: code[3] is 1-4 or A-D
    weight FLOAT NOT NULL,            -- 0.5 if code[6] is H, 1.0 if Y
    campus TEXT NOT NULL,             -- code[7]: 0 online, 1 utsg, 3 utsc, 5 utm
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    prerequisites TEXT,               -- requisite JSON, or NULL if none
    corequisites TEXT,                -- requisite JSON, or NULL if none
    exclusions TEXT,                  -- exclusions JSON, or NULL if none
    breadth TEXT                      -- breadth JSON, or NULL if none
) WITHOUT ROWID;

CREATE INDEX idx_course_designator ON courses(designator);
CREATE INDEX idx_course_level ON courses(level);
CREATE INDEX idx_course_weight ON courses(weight);
CREATE INDEX idx_course_campus ON courses(campus);
```

Example row:

```
         code = DRM228H1
   designator = DRM
       number = 228
        level = 200
       weight = 0.5
       campus = utsg
        title = Playwriting I
  description = A hands-on study of the craft of dramatic writing. The class examines the basic elements of playwriting such as plot, structure, theme, character, dialogue, setting, with an emphasis on story-making. Attention is given to the development of students own work through written assignments and in-class exercises. Priority will be given to students in the Drama Specialist and Major.
prerequisites = {"req":{"type":"and","items":[{"type":"program","programs":["ASMAJ2148","ASSPE2148","ASMIN2148"],"text":"Drama Major, Specialist or Minor"},{"type":"course","code":"DRM101Y1"}]},"raw":"Drama Major, Specialist or Minor;  DRM101Y1"}
 corequisites = {"req":{"type":"course","code":"DRM220Y1"},"raw":"DRM220Y1"}
   exclusions = {"items":[{"courses":["DRE362H5"]}],"raw":"DRE362H5"}
      breadth = [1]
```

<details>
<summary>The same row as JSON</summary>

```json
{
  "code": "DRM228H1",
  "designator": "DRM",
  "number": "228",
  "level": 200,
  "weight": 0.5,
  "campus": "utsg",
  "title": "Playwriting I",
  "description": "A hands-on study of the craft of dramatic writing. The class examines the basic elements of playwriting such as plot, structure, theme, character, dialogue, setting, with an emphasis on story-making. Attention is given to the development of students own work through written assignments and in-class exercises. Priority will be given to students in the Drama Specialist and Major.",
  "prerequisites": {
    "req": {
      "type": "and",
      "items": [
        {
          "type": "program",
          "programs": [
            "ASMAJ2148",
            "ASSPE2148",
            "ASMIN2148"
          ],
          "text": "Drama Major, Specialist or Minor"
        },
        {
          "type": "course",
          "code": "DRM101Y1"
        }
      ]
    },
    "raw": "Drama Major, Specialist or Minor;  DRM101Y1"
  },
  "corequisites": {
    "req": {
      "type": "course",
      "code": "DRM220Y1"
    },
    "raw": "DRM220Y1"
  },
  "exclusions": {
    "items": [
      {
        "courses": [
          "DRE362H5"
        ]
      }
    ],
    "raw": "DRE362H5"
  },
  "breadth": [
    1
  ]
}
```

</details>

#### Prerequisites and corequisites

`req` is the requirement tree, or `null` when the course requires nothing, for example `{"req": null, "notes": ["NA"], "raw": "NA"}`.

<details>
<summary>JSON Schema and Field Types</summary>

```json
{
  "$id": "urn:dgx:requisite",
  "type": "object",
  "required": ["req", "raw"],
  "additionalProperties": false,
  "properties": {
    "req": {"anyOf": [{"$ref": "urn:dgx:node"}, {"type": "null"}]},
    "notes": {"type": "array", "items": {"type": "string"}},
    "raw": {"type": "string"}
  }
}
```

| Field | Type | Required |
|---|---|---|
| `req` | `node` or `null` | Yes |
| `notes` | `string[]` | No |
| `raw` | `string` | Yes |

</details>

#### Exclusions

Each item in `items` is one way to be excluded. A match on an item with a `note` is Review.

| Item | The student is excluded if | Example |
|---|---|---|
| Course set | They took every course in `courses`. | `{"courses": ["JCC250H1"]}` |
| Concurrent course set | They're taking the courses in `courses`. | `{"courses": ["AST425Y1"], "concurrent": true}` |
| Filter | They took a course with one of `designators`, at one of `levels` (any level if empty), and not in `exclude`. | `{"designators": ["FSL"], "levels": [400]}` |
| Filter with a note | The filter matches. The result is Review. | `{"designators": ["CSC"], "exclude": ["CSC196H1", "CSC197H1", "CSC199H1"], "note": "except AP, IB, CAPE or GCE Transfer Credits"}` |
| Text | Never decided automatically. The item is Review. | `{"kind": "preparation", "text": "OAC Hebrew"}` |

`raw` is empty when the course has no exclusions text, and the items come from a timing restriction in its requisites.

<details>
<summary>JSON Schema and Field Types</summary>

```json
{
  "$id": "urn:dgx:exclusions",
  "type": "object",
  "required": ["items", "raw"],
  "additionalProperties": false,
  "properties": {
    "items": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "properties": {
          "courses": {"type": "array", "items": {"type": "string"}},
          "designators": {"type": "array", "items": {"type": "string"}},
          "levels": {"type": "array", "items": {"enum": [100, 200, 300, 400]}},
          "exclude": {"type": "array", "items": {"type": "string"}},
          "concurrent": {"type": "boolean"},
          "note": {"type": "string"},
          "kind": {"$ref": "urn:dgx:node#/properties/kind"},
          "text": {"type": "string"}
        }
      }
    },
    "notes": {"type": "array", "items": {"type": "string"}},
    "raw": {"type": "string"}
  }
}
```

| Field | Type | Required |
|---|---|---|
| `items` | `object[]` | Yes |
| `items[].courses` | `string[]` | No |
| `items[].designators` | `string[]` | No |
| `items[].levels` | `integer[]` | No |
| `items[].exclude` | `string[]` | No |
| `items[].concurrent` | `boolean` | No |
| `items[].note` | `string` | No |
| `items[].kind` | `string` | No |
| `items[].text` | `string` | No |
| `notes` | `string[]` | No |
| `raw` | `string` | Yes |

</details>

#### Breadth

An array of breadth category numbers, for example `[1, 3]`:

| Category | Name |
|---|---|
| `1` | Creative and Cultural Representations |
| `2` | Thought, Belief and Behaviour |
| `3` | Society and its Institutions |
| `4` | Living Things and Their Environment |
| `5` | The Physical and Mathematical Universes |

<details>
<summary>JSON Schema and Field Types</summary>

```json
{
  "$id": "urn:dgx:breadth",
  "type": "array",
  "items": {"enum": [1, 2, 3, 4, 5]}
}
```

| Field | Type | Required |
|---|---|---|
| `[]` | `integer`, 1 to 5 | Yes |

</details>

### Programs table

```sql
CREATE TABLE programs (
    code TEXT PRIMARY KEY NOT NULL,   -- ASMIN1001
    type TEXT NOT NULL,               -- Specialist, Major, Minor, Focus, or Certificate
    number TEXT NOT NULL,             -- 1001
    title TEXT NOT NULL,
    enrolment TEXT,                   -- enrolment JSON, or NULL if none
    completion TEXT,                  -- completion JSON, or NULL if none
    areas TEXT                        -- areas JSON, or NULL if none
) WITHOUT ROWID;

CREATE INDEX idx_program_type ON programs(type);
```

Example row:

```
      code = ASMIN1001
      type = Minor
    number = 1001
     title = Bioethics Minor (Arts Program)
 enrolment = {"kind":"open","req":{"type":"credits","amount":4},"raw":"This is an open enrolment program. A student who has completed 4.0 credits may enrol in the program."}
completion = {"requirements":[{"number":1,"kind":"allocation","req":{"type":"credits","amount":4,"designators":["PHL"]},"raw":"(4.0 credits including at least 1.0 credit at the 300+ level)\n5. Additional philosophy courses to a total of 4.0 credits"},{"number":2,"kind":"constraint","parent":1,"req":{"type":"credits","amount":1,"levels":[300,400]},"raw":"including at least 1.0 credit at the 300+ level"},{"number":3,"kind":"allocation","label":"1","parent":1,"req":{"type":"course","code":"PHL281H1"},"raw":"1. PHL281H1"},{"number":4,"kind":"allocation","label":"2","parent":1,"req":{"type":"or","items":[{"type":"course","code":"PHL245H1"},{"type":"course","code":"PHL246H1"}]},"raw":"2. PHL245H1/ PHL246H1"},{"number":5,"kind":"allocation","label":"3","parent":1,"req":{"type":"credits","amount":0.5,"lists":["Group 1"]},"raw":"3. 0.5 credit from Group 1 (Advanced Bioethics)"},{"number":6,"kind":"allocation","label":"4","parent":1,"req":{"type":"credits","amount":1,"lists":["Requirement 4 courses"]},"raw":"4. 1.0 credit from PHL271H1 / PHL275H1 / Group 2 (Value Theory)"}],"lists":[{"name":"Group 1","title":"Group 1 - Advanced Bioethics","items":[{"type":"credits","courses":["PHL380H1","PHL381H1","PHL382H1","PHL383H1","PHL384H1","PHL440H1","PHL470H1"]}]},{"name":"Group 2","title":"Group 2 - Value Theory","items":[{"type":"credits","courses":["PHL265H1","PHL295H1","PHL365H1","PHL366H1","PHL370H1","PHL375H1","PHL407H1","PHL412H1","PHL413H1","PHL483H1"]}]},{"name":"Requirement 4 courses","items":[{"type":"credits","courses":["PHL271H1","PHL275H1"]},{"type":"credits","lists":["Group 2"]}]}],"raw":"(4.0 credits including at least 1.0 credit at the 300+ level)\n1. PHL281H1\n2. PHL245H1/ PHL246H1\n3. 0.5 credit from Group 1 (Advanced Bioethics)\n4. 1.0 credit from PHL271H1 / PHL275H1 / Group 2 (Value Theory)\n5. Additional philosophy courses to a total of 4.0 credits\n_Group 1 - Advanced Bioethics:_\nPHL380H1, PHL381H1, PHL382H1, PHL383H1, PHL384H1, PHL440H1, PHL470H1\n_Group 2 - Value Theory:_\nPHL265H1, PHL295H1, PHL365H1, PHL366H1, PHL370H1, PHL375H1, PHL407H1, PHL412H1, PHL413H1, PHL483H1"}
     areas = ["Philosophy"]
```

<details>
<summary>The same row as JSON</summary>

```json
{
  "code": "ASMIN1001",
  "type": "Minor",
  "number": "1001",
  "title": "Bioethics Minor (Arts Program)",
  "enrolment": {
    "kind": "open",
    "req": {
      "type": "credits",
      "amount": 4
    },
    "raw": "This is an open enrolment program. A student who has completed 4.0 credits may enrol in the program."
  },
  "completion": {
    "requirements": [
      {
        "number": 1,
        "kind": "allocation",
        "req": {
          "type": "credits",
          "amount": 4,
          "designators": [
            "PHL"
          ]
        },
        "raw": "(4.0 credits including at least 1.0 credit at the 300+ level)\n5. Additional philosophy courses to a total of 4.0 credits"
      },
      {
        "number": 2,
        "kind": "constraint",
        "parent": 1,
        "req": {
          "type": "credits",
          "amount": 1,
          "levels": [
            300,
            400
          ]
        },
        "raw": "including at least 1.0 credit at the 300+ level"
      },
      {
        "number": 3,
        "kind": "allocation",
        "label": "1",
        "parent": 1,
        "req": {
          "type": "course",
          "code": "PHL281H1"
        },
        "raw": "1. PHL281H1"
      },
      {
        "number": 4,
        "kind": "allocation",
        "label": "2",
        "parent": 1,
        "req": {
          "type": "or",
          "items": [
            {
              "type": "course",
              "code": "PHL245H1"
            },
            {
              "type": "course",
              "code": "PHL246H1"
            }
          ]
        },
        "raw": "2. PHL245H1/ PHL246H1"
      },
      {
        "number": 5,
        "kind": "allocation",
        "label": "3",
        "parent": 1,
        "req": {
          "type": "credits",
          "amount": 0.5,
          "lists": [
            "Group 1"
          ]
        },
        "raw": "3. 0.5 credit from Group 1 (Advanced Bioethics)"
      },
      {
        "number": 6,
        "kind": "allocation",
        "label": "4",
        "parent": 1,
        "req": {
          "type": "credits",
          "amount": 1,
          "lists": [
            "Requirement 4 courses"
          ]
        },
        "raw": "4. 1.0 credit from PHL271H1 / PHL275H1 / Group 2 (Value Theory)"
      }
    ],
    "lists": [
      {
        "name": "Group 1",
        "title": "Group 1 - Advanced Bioethics",
        "items": [
          {
            "type": "credits",
            "courses": [
              "PHL380H1",
              "PHL381H1",
              "PHL382H1",
              "PHL383H1",
              "PHL384H1",
              "PHL440H1",
              "PHL470H1"
            ]
          }
        ]
      },
      {
        "name": "Group 2",
        "title": "Group 2 - Value Theory",
        "items": [
          {
            "type": "credits",
            "courses": [
              "PHL265H1",
              "PHL295H1",
              "PHL365H1",
              "PHL366H1",
              "PHL370H1",
              "PHL375H1",
              "PHL407H1",
              "PHL412H1",
              "PHL413H1",
              "PHL483H1"
            ]
          }
        ]
      },
      {
        "name": "Requirement 4 courses",
        "items": [
          {
            "type": "credits",
            "courses": [
              "PHL271H1",
              "PHL275H1"
            ]
          },
          {
            "type": "credits",
            "lists": [
              "Group 2"
            ]
          }
        ]
      }
    ],
    "raw": "(4.0 credits including at least 1.0 credit at the 300+ level)\n1. PHL281H1\n2. PHL245H1/ PHL246H1\n3. 0.5 credit from Group 1 (Advanced Bioethics)\n4. 1.0 credit from PHL271H1 / PHL275H1 / Group 2 (Value Theory)\n5. Additional philosophy courses to a total of 4.0 credits\n_Group 1 - Advanced Bioethics:_\nPHL380H1, PHL381H1, PHL382H1, PHL383H1, PHL384H1, PHL440H1, PHL470H1\n_Group 2 - Value Theory:_\nPHL265H1, PHL295H1, PHL365H1, PHL366H1, PHL370H1, PHL375H1, PHL407H1, PHL412H1, PHL413H1, PHL483H1"
  },
  "areas": [
    "Philosophy"
  ]
}
```

</details>

#### Enrolment

| `kind` | If `req` is met | Example `req` |
|---|---|---|
| `open` | The student can enrol. | `{"type": "credits", "amount": 4}` |
| `limited` | The student can apply. Admission isn't guaranteed. | `{"type": "and", "items": [{"type": "credits", "amount": 4}, {"type": "course", "code": "BMS100H1", "min_grade": 70}]}` |
| `focus` | The student can enrol in this focus. `req` names the parent program. | `{"type": "program", "programs": ["ASMAJ0652"], "text": "History Major"}` |
| `suspended` | Never. The program isn't admitting. | `null` |

`exclusions` lists programs that the student can't be enrolled in, for example `[{"type": "program", "programs": ["ASMAJ0552", "ASMIN0552"], "text": "Major or Minor in Latin American Studies"}]`. `first_round_only: true` means that the program considers requests only in the first program request period.

<details>
<summary>JSON Schema and Field Types</summary>

```json
{
  "$id": "urn:dgx:enrolment",
  "type": "object",
  "required": ["kind", "req", "raw"],
  "additionalProperties": false,
  "properties": {
    "kind": {"enum": ["open", "limited", "focus", "suspended"]},
    "req": {"anyOf": [{"$ref": "urn:dgx:node"}, {"type": "null"}]},
    "exclusions": {"type": "array", "items": {"$ref": "urn:dgx:node"}},
    "first_round_only": {"type": "boolean"},
    "notes": {"type": "array", "items": {"type": "string"}},
    "raw": {"type": "string"}
  }
}
```

| Field | Type | Required |
|---|---|---|
| `kind` | `string` | Yes |
| `req` | `node` or `null` | Yes |
| `exclusions` | `node[]` | No |
| `first_round_only` | `boolean` | No |
| `notes` | `string[]` | No |
| `raw` | `string` | Yes |

</details>

#### Completion

`requirements` holds Degree Explorer's numbered rows, 1 to N without gaps. A row with `parent` belongs to that allocation row. A row without `parent` is top level.

| Row `kind` | Meaning | Example |
|---|---|---|
| `allocation` | Claims courses for its `req`. | `"kind": "allocation", "parent": 1, "req": {"type": "course", "code": "PHL281H1"}` |
| `constraint` | A floor on the courses counted under `parent`. Claims nothing. | `"kind": "constraint", "parent": 1, "req": {"type": "credits", "amount": 1, "levels": [300, 400]}` |
| `limit` | At most `max_amount` matching credits count toward `parent`. Claims nothing. | `"kind": "limit", "parent": 1, "req": {"type": "credits", "levels": [100], "max_amount": 1, ...}` |
| `choice` | Picks among option rows through `requirement` nodes. Top level only. Claims nothing. | `"kind": "choice", "req": {"type": "or", "items": [{"type": "requirement", "requirements": [17]}, ...]}` |

| Row field | Meaning | Example |
|---|---|---|
| `label` | The calendar's own label for the row. | `"label": "1"` |
| `section` | The heading the row sits under. Display only. | `"section": "First Year"` |
| `overlaps` | Allocations under the same parent that can share a course with this row. | `"overlaps": [4]` |
| `req` | `null` for an option group, which is met when its child rows are met. | `"req": null` |
| `audience` | A `profile` node. The row applies only to students who match it. | `"audience": {"type": "profile", "attr": "faculty", "value": "Arts & Science", ...}` |
| `scope` | `transcript`: a constraint or limit counts over every course taken, not only the parent's. | `"scope": "transcript"` |

A course is on a list in `lists` if it matches any of the list's items:

| List item | Example |
|---|---|
| A `credits` filter. | `{"type": "credits", "lists": ["Group 2"]}` |
| A pair of courses that count only together. | `{"type": "and", "items": [{"type": "course", "code": "PHL205H1"}, {"type": "course", "code": "PHL206H1"}]}` |
| A course that might count: `and(filter, text)`. | `{"type": "and", "items": [{"type": "credits"}, {"type": "text", "kind": "content", "text": "offered by the Canadian Studies Program"}]}` |

`exclusions` lists programs that a student can't combine with this one, for example `[{"type": "program", "programs": ["ASMAJ1400", "ASSPE1400"], "text": "German Specialist or Major"}]`.

<details>
<summary>JSON Schema and Field Types</summary>

```json
{
  "$id": "urn:dgx:completion",
  "type": "object",
  "required": ["requirements", "raw"],
  "additionalProperties": false,
  "properties": {
    "requirements": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["number", "kind", "req", "raw"],
        "additionalProperties": false,
        "properties": {
          "number": {"type": "integer", "minimum": 1},
          "kind": {"enum": ["allocation", "constraint", "limit", "choice"]},
          "label": {"type": "string"},
          "section": {"type": "string"},
          "parent": {"type": "integer", "minimum": 1},
          "overlaps": {"type": "array", "items": {"type": "integer", "minimum": 1}},
          "req": {"anyOf": [{"$ref": "urn:dgx:node"}, {"type": "null"}]},
          "audience": {"$ref": "urn:dgx:node"},
          "scope": {"enum": ["transcript"]},
          "notes": {"type": "array", "items": {"type": "string"}},
          "raw": {"type": "string"}
        }
      }
    },
    "lists": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["name", "items"],
        "additionalProperties": false,
        "properties": {
          "name": {"type": "string"},
          "title": {"type": "string"},
          "items": {"type": "array", "items": {"$ref": "urn:dgx:node"}}
        }
      }
    },
    "exclusions": {"type": "array", "items": {"$ref": "urn:dgx:node"}},
    "notes": {"type": "array", "items": {"type": "string"}},
    "raw": {"type": "string"}
  }
}
```

| Field | Type | Required |
|---|---|---|
| `requirements` | `object[]` | Yes |
| `requirements[].number` | `integer` | Yes |
| `requirements[].kind` | `string` | Yes |
| `requirements[].label` | `string` | No |
| `requirements[].section` | `string` | No |
| `requirements[].parent` | `integer` | No |
| `requirements[].overlaps` | `integer[]` | No |
| `requirements[].req` | `node` or `null` | Yes |
| `requirements[].audience` | `node` | No |
| `requirements[].scope` | `string` | No |
| `requirements[].notes` | `string[]` | No |
| `requirements[].raw` | `string` | Yes |
| `lists` | `object[]` | No |
| `lists[].name` | `string` | Yes |
| `lists[].title` | `string` | No |
| `lists[].items` | `node[]` | Yes |
| `exclusions` | `node[]` | No |
| `notes` | `string[]` | No |
| `raw` | `string` | Yes |

</details>

#### Areas

An array of subject area names, for example `["Computer Science", "Statistical Sciences"]`.

<details>
<summary>JSON Schema and Field Types</summary>

```json
{
  "$id": "urn:dgx:areas",
  "type": "array",
  "items": {"type": "string"}
}
```

| Field | Type | Required |
|---|---|---|
| `[]` | `string` | Yes |

</details>

### Version table

The table has one row per school year scraped. The update job compares the highest `school_year` with the current school year, which starts in June. If the calendar site rate-limits a download, the job waits and retries. Any other failure ends the run without storing anything, and the next scheduled run tries again.

```sql
CREATE TABLE version (
    school_year TEXT PRIMARY KEY NOT NULL,   -- "2026-2027"
    last_updated TEXT NOT NULL               -- RFC 3339, UTC
) WITHOUT ROWID;
```

Example row:

```
 school_year = 2026-2027
last_updated = 2026-10-17T22:26:23Z
```

The same row as JSON:

```json
{
  "school_year": "2026-2027",
  "last_updated": "2026-10-17T22:26:23Z"
}
```

---

## AI disclaimer

Claude Opus 5.5 helped write the parser package, particularly the regex patterns built from raw page data. Claude also helped test and harden the code, and write both comments throughout the codebase and this README.

All other code in this repository was written by a human.
