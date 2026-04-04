package model

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OrderField struct {
	Table string
	Name  string
	Desc  bool
}

func BuildOrderBy(fields ...OrderField) clause.OrderBy {
	columns := make([]clause.OrderByColumn, 0, len(fields))
	for _, field := range fields {
		if field.Name == "" {
			continue
		}
		columns = append(columns, clause.OrderByColumn{
			Column: clause.Column{
				Table: field.Table,
				Name:  field.Name,
			},
			Desc: field.Desc,
		})
	}
	return clause.OrderBy{Columns: columns}
}

func ApplyOrderBy(db *gorm.DB, fields ...OrderField) *gorm.DB {
	if db == nil {
		return nil
	}
	return db.Clauses(BuildOrderBy(fields...))
}
