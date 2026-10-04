package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

type Course struct {
	Code            string  `json:"code"`       // ex. CSC148H1
	Designator      string  `json:"designator"` // ex. CSC
	Number          string  `json:"number"`     // ex. 148
	Level           int64   `json:"level"`      // 100-400
	Weight          float64 `json:"weight"`     // 0.5 or 1.0
	Campus          int64   `json:"campus"`     // 0, 1, 3, or 5
	Title           string  `json:"title"`
	Prerequisites   *string `json:"prerequisites"`
	Corequisites    *string `json:"corequisites"`
	Exclusions      *string `json:"exclusions"`
	BreadthReq      *string `json:"breadth_req"`
	DistributionReq *string `json:"distribution_req"`
}

func (s *Store) CreateCourse(ctx context.Context, c *Course) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO courses (code, designator, number, level, weight, campus, title, prerequisites, corequisites, exclusions, breadth_req, distribution_req)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, )`,
		c.Code, c.Designator, c.Number, c.Level, c.Weight, c.Campus, c.Title,
		c.Prerequisites, c.Corequisites, c.Exclusions, c.BreadthReq, c.DistributionReq,
	)
	if err != nil {
		return fmt.Errorf("insert course: %w", err)
	}
	return nil
}

func (s *Store) GetCourse(ctx context.Context, courseCode string) (Course, error) {
	var c Course
	err := s.db.QueryRowContext(ctx,
		`SELECT code, designator, number, level, weight, campus, title, prerequisites, corequisites, exclusions, breadth_req, distribution_req
		FROM courses WHERE code = ?`, courseCode,
	).Scan(&c.Code, &c.Designator, &c.Number, &c.Level, &c.Weight, &c.Campus, &c.Title,
		&c.Prerequisites, &c.Corequisites, &c.Exclusions, &c.BreadthReq, &c.DistributionReq,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Course{}, ErrNotFound
	}
	if err != nil {
		return Course{}, fmt.Errorf("get course %s: %w", courseCode, err)
	}
	return c, nil
}

func (s *Store) SearchCourses(ctx context.Context, q string, limit int) ([]Course, error) {
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

	var courses []Course
	for rows.Next() {
		var c Course
		if err := rows.Scan(
			&c.Code, &c.Designator, &c.Number, &c.Level, &c.Weight, &c.Campus, &c.Title,
			&c.Prerequisites, &c.Corequisites, &c.Exclusions, &c.BreadthReq, &c.DistributionReq,
		); err != nil {
			return nil, fmt.Errorf("scan course: %w", err)
		}
		courses = append(courses, c)
	}
	return courses, rows.Err()
}

func (s *Store) UpdateCourse(ctx context.Context, c Course) error {
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
