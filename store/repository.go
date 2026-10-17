package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"degree-explorer/types"
)

var ErrNotFound = errors.New("not found")

const upsertCourseSQL = `INSERT INTO courses (code, designator, number, level, weight, campus, title, description, prerequisites, corequisites, exclusions, breadth)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(code) DO UPDATE SET
			designator = excluded.designator,
			number = excluded.number,
			level = excluded.level,
			weight = excluded.weight,
			campus = excluded.campus,
			title = excluded.title,
			description = excluded.description,
			prerequisites = excluded.prerequisites,
			corequisites = excluded.corequisites,
			exclusions = excluded.exclusions,
			breadth = excluded.breadth`

const upsertProgramSQL = `INSERT INTO programs (code, type, number, title, enrolment, completion, areas)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(code) DO UPDATE SET
			type = excluded.type,
			number = excluded.number,
			title = excluded.title,
			enrolment = excluded.enrolment,
			completion = excluded.completion,
			areas = excluded.areas`

// ----------------------------------------------------------------------------
// Courses
// ----------------------------------------------------------------------------

func (s *Store) CreateCourse(ctx context.Context, c *types.Course) error {
	_, err := s.db.ExecContext(ctx, upsertCourseSQL,
		c.Code, c.Designator, c.Number, c.Level, c.Weight, c.Campus, c.Title, c.Description,
		c.Prerequisites, c.Corequisites, c.Exclusions, c.Breadth,
	)
	if err != nil {
		return fmt.Errorf("insert course: %w", err)
	}
	return nil
}

func (s *Store) CreateCourses(ctx context.Context, cl []*types.Course) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, upsertCourseSQL)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, c := range cl {
		if _, err := stmt.ExecContext(ctx,
			c.Code, c.Designator, c.Number, c.Level, c.Weight, c.Campus, c.Title, c.Description,
			c.Prerequisites, c.Corequisites, c.Exclusions, c.Breadth,
		); err != nil {
			return fmt.Errorf("insert course %s: %w", c.Code, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *Store) GetCourse(ctx context.Context, courseCode string) (types.Course, error) {
	var c types.Course
	err := s.db.QueryRowContext(ctx,
		`SELECT code, designator, number, level, weight, campus, title, description, prerequisites, corequisites, exclusions, breadth
		FROM courses WHERE code = ?`, courseCode,
	).Scan(&c.Code, &c.Designator, &c.Number, &c.Level, &c.Weight, &c.Campus, &c.Title, &c.Description,
		&c.Prerequisites, &c.Corequisites, &c.Exclusions, &c.Breadth,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return types.Course{}, fmt.Errorf("get course %s: %w", courseCode, ErrNotFound)
	}
	if err != nil {
		return types.Course{}, fmt.Errorf("get course %s: %w", courseCode, err)
	}
	return c, nil
}

func (s *Store) SearchCourses(ctx context.Context, q string, limit int) ([]types.Course, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT code, designator, number, level, weight, campus, title, description, prerequisites, corequisites, exclusions, breadth
		FROM courses
		WHERE code LIKE ?
		ORDER BY code
		LIMIT ?`,
		"%"+q+"%", limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search courses (query: %s): %w", q, err)
	}
	defer rows.Close()

	var results []types.Course
	for rows.Next() {
		var c types.Course
		if err := rows.Scan(
			&c.Code, &c.Designator, &c.Number, &c.Level, &c.Weight, &c.Campus, &c.Title, &c.Description,
			&c.Prerequisites, &c.Corequisites, &c.Exclusions, &c.Breadth,
		); err != nil {
			return nil, fmt.Errorf("scan course: %w", err)
		}
		results = append(results, c)
	}
	return results, rows.Err()
}

func (s *Store) UpdateCourse(ctx context.Context, c types.Course) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE courses
		SET title = ?, description = ?, prerequisites = ?, corequisites = ?, exclusions = ?, breadth = ?
		WHERE code = ?`,
		c.Title, c.Description, c.Prerequisites, c.Corequisites,
		c.Exclusions, c.Breadth, c.Code,
	)
	if err != nil {
		return fmt.Errorf("update course %s: %w", c.Code, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteCourse(ctx context.Context, courseCode string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM courses WHERE code = ?`,
		courseCode,
	)
	if err != nil {
		return fmt.Errorf("delete course %s: %w", courseCode, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ----------------------------------------------------------------------------
// Programs
// ----------------------------------------------------------------------------

func (s *Store) CreateProgram(ctx context.Context, p *types.Program) error {
	_, err := s.db.ExecContext(ctx, upsertProgramSQL,
		p.Code, p.Type, p.Number, p.Title, p.Enrolment, p.Completion, p.Areas,
	)
	if err != nil {
		return fmt.Errorf("insert program: %w", err)
	}
	return nil
}

func (s *Store) CreatePrograms(ctx context.Context, pl []*types.Program) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, upsertProgramSQL)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, p := range pl {
		if _, err := stmt.ExecContext(ctx,
			p.Code, p.Type, p.Number, p.Title, p.Enrolment, p.Completion, p.Areas,
		); err != nil {
			return fmt.Errorf("insert program %s: %w", p.Code, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *Store) GetProgram(ctx context.Context, programCode string) (types.Program, error) {
	var p types.Program
	err := s.db.QueryRowContext(ctx,
		`SELECT code, type, number, title, enrolment, completion, areas
		FROM programs WHERE code = ?`, programCode,
	).Scan(&p.Code, &p.Type, &p.Number, &p.Title, &p.Enrolment, &p.Completion, &p.Areas)

	if errors.Is(err, sql.ErrNoRows) {
		return types.Program{}, fmt.Errorf("get program %s: %w", programCode, ErrNotFound)
	}
	if err != nil {
		return types.Program{}, fmt.Errorf("get program %s: %w", programCode, err)
	}
	return p, nil
}

func (s *Store) SearchPrograms(ctx context.Context, q string, limit int) ([]types.Program, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT code, type, number, title, enrolment, completion, areas
		FROM programs
		WHERE code LIKE ?
		ORDER BY code
		LIMIT ?`,
		"%"+q+"%", limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search programs (query: %s): %w", q, err)
	}
	defer rows.Close()

	var results []types.Program
	for rows.Next() {
		var p types.Program
		if err := rows.Scan(
			&p.Code, &p.Type, &p.Number, &p.Title, &p.Enrolment, &p.Completion, &p.Areas,
		); err != nil {
			return nil, fmt.Errorf("scan program: %w", err)
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

func (s *Store) UpdateProgram(ctx context.Context, p types.Program) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE programs
		SET title = ?, enrolment = ?, completion = ?, areas = ?
		WHERE code = ?`,
		p.Title, p.Enrolment, p.Completion, p.Areas, p.Code,
	)
	if err != nil {
		return fmt.Errorf("update program %s: %w", p.Code, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteProgram(ctx context.Context, programCode string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM programs WHERE code = ?`,
		programCode,
	)
	if err != nil {
		return fmt.Errorf("delete program %s: %w", programCode, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ----------------------------------------------------------------------------
// Version
// ----------------------------------------------------------------------------

// SchoolYear returns the latest school year the db was updated for, or "" if none.
func (s *Store) SchoolYear(ctx context.Context) (string, error) {
	var year string
	err := s.db.QueryRowContext(ctx,
		`SELECT school_year FROM version ORDER BY school_year DESC LIMIT 1`,
	).Scan(&year)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get school year: %w", err)
	}
	return year, nil
}

func (s *Store) SetVersion(ctx context.Context, schoolYear string, lastUpdated time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO version (school_year, last_updated) VALUES (?, ?)
		ON CONFLICT(school_year) DO UPDATE SET last_updated = excluded.last_updated`,
		schoolYear, lastUpdated.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("set version %s: %w", schoolYear, err)
	}
	return nil
}
