package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"degree-explorer/types"
)

var ErrNotFound = errors.New("not found")

func (s *Store) CreateCourse(ctx context.Context, c *types.Course) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO courses (code, designator, number, level, weight, campus, title, prerequisites, corequisites, exclusions, breadth_req, distribution_req)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Code, c.Designator, c.Number, c.Level, c.Weight, c.Campus, c.Title,
		c.Prerequisites, c.Corequisites, c.Exclusions, c.BreadthReq, c.DistributionReq,
	)
	if err != nil {
		return fmt.Errorf("insert course: %w", err)
	}
	return nil
}

func (s *Store) GetCourse(ctx context.Context, courseCode string) (types.Course, error) {
	var c types.Course
	err := s.db.QueryRowContext(ctx,
		`SELECT code, designator, number, level, weight, campus, title, prerequisites, corequisites, exclusions, breadth_req, distribution_req
		FROM courses WHERE code = ?`, courseCode,
	).Scan(&c.Code, &c.Designator, &c.Number, &c.Level, &c.Weight, &c.Campus, &c.Title,
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
		`SELECT code, designator, number, level, weight, campus, title, prerequisites, corequisites, exclusions, breadth_req, distribution_req
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
			&c.Code, &c.Designator, &c.Number, &c.Level, &c.Weight, &c.Campus, &c.Title,
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
		SET title = ?, prerequisites = ?, corequisites = ?, exclusions = ?, breadth_req = ?, distribution_req = ?
		WHERE code = ?`,
		c.Title, c.Prerequisites, c.Corequisites, c.Exclusions, c.BreadthReq, c.DistributionReq,
		c.Code,
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
