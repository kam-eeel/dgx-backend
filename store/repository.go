package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"degree-explorer/types"
)

var ErrNotFound = errors.New("not found")

const upsertCourseSQL = `INSERT INTO courses (code, designator, number, level, weight, campus, title, description, prerequisites, corequisites, exclusions, breadth_req, distribution_req)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			breadth_req = excluded.breadth_req,
			distribution_req = excluded.distribution_req`

func (s *Store) CreateCourse(ctx context.Context, c *types.Course) error {
	_, err := s.db.ExecContext(ctx, upsertCourseSQL,
		c.Code, c.Designator, c.Number, c.Level, c.Weight, c.Campus, c.Title, c.Description,
		c.Prerequisites, c.Corequisites, c.Exclusions, c.BreadthReq, c.DistributionReq,
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
			c.Prerequisites, c.Corequisites, c.Exclusions, c.BreadthReq, c.DistributionReq,
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
		`SELECT code, designator, number, level, weight, campus, title, description, prerequisites, corequisites, exclusions, breadth_req, distribution_req
		FROM courses WHERE code = ?`, courseCode,
	).Scan(&c.Code, &c.Designator, &c.Number, &c.Level, &c.Weight, &c.Campus, &c.Title, &c.Description,
		&c.Prerequisites, &c.Corequisites, &c.Exclusions, &c.BreadthReq, &c.DistributionReq,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return types.Course{}, ErrNotFound
	}
	if err != nil {
		return types.Course{}, fmt.Errorf("get course %s: %w", courseCode, err)
	}
	return c, nil
}

func (s *Store) SearchCourses(ctx context.Context, q string, limit int) ([]types.Course, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT code, designator, number, level, weight, campus, title, description, prerequisites, corequisites, exclusions, breadth_req, distribution_req
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
			&c.Prerequisites, &c.Corequisites, &c.Exclusions, &c.BreadthReq, &c.DistributionReq,
		); err != nil {
			return nil, fmt.Errorf("scan course: %w", err)
		}
		results = append(results, c)
	}
	return results, rows.Err()
}

func (s *Store) UpdateCourse(ctx context.Context, c types.Course) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE COURSES
		SET title = ?, description = ?, prerequisites = ?, corequisites = ?, exclusions = ?, breadth_req = ?, distribution_req = ?
		WHERE code = ?`,
		c.Title, c.Description, c.Prerequisites, c.Corequisites,
		c.Exclusions, c.BreadthReq, c.DistributionReq, c.Code,
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
