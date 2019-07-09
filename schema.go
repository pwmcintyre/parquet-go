package go_parquet

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/fraugster/parquet-go/parquet"
)

type element interface {
	create(schema []*parquet.SchemaElement, name string, flatMap map[string]Column, index, dLevel, rLevel int) (int, error)
}

type Column interface {
	Name() string
	MaxDefinitionLevel() uint16
	MaxRepetitionLevel() uint16
	Element() *parquet.SchemaElement
}

// Schema is the schema reader/creator for parquet schema
type Schema struct {
	root group

	flatMap map[string]Column
}

type group struct {
	*parquet.SchemaElement
	dLevel, rLevel int

	children []element
}

type primitive struct {
	dLevel, rLevel int
	flatName       string

	*parquet.SchemaElement
}

func (s *Schema) Columns() map[string]Column {
	return s.flatMap
}

func (s *Schema) GetColumnByName(path string) (Column, bool) {
	c, ok := s.flatMap[path]
	return c, ok
}

func (p *primitive) Name() string {
	return p.SchemaElement.Name
}

func (p *primitive) MaxDefinitionLevel() uint16 {
	if p.dLevel > 65535 {
		panic("out of range for definition level")
	}
	return uint16(p.dLevel)
}

func (p *primitive) MaxRepetitionLevel() uint16 {
	if p.rLevel > 65535 {
		panic("out of range for repetition level")
	}
	return uint16(p.rLevel)
}

func (p *primitive) Element() *parquet.SchemaElement {
	return p.SchemaElement
}

func (g *group) create(schema []*parquet.SchemaElement, name string, flatMap map[string]Column, idx, dLevel, rLevel int) (int, error) {
	if len(schema) <= idx {
		return 0, errors.New("schema index out of bound")
	}

	s := schema[idx]
	if s.Type != nil {
		return 0, errors.Errorf("field Type is not nil in index %d", idx)
	}
	if s.NumChildren == nil {
		return 0, errors.Errorf("the field NumChildren is invalid in index %d", idx)
	}

	if *s.NumChildren <= 0 {
		return 0, errors.Errorf("the field NumChildren is zero in index %d", idx)
	}
	l := int(*s.NumChildren)

	if len(schema) <= idx+l {
		return 0, errors.Errorf("not enough element in the schema list in index %d", idx)
	}

	if s.RepetitionType != nil && *s.RepetitionType != parquet.FieldRepetitionType_REQUIRED {
		dLevel++
	}

	if s.RepetitionType != nil && *s.RepetitionType == parquet.FieldRepetitionType_REPEATED {
		rLevel++
	}

	if idx != 0 {
		if name == "" {
			name = s.Name
		} else {
			name += "." + s.Name
		}
	}

	// TODO : Do more validation here
	g.SchemaElement = s
	g.children = make([]element, 0, l)

	var err error
	for i := 0; i < l; i++ {
		idx++
		if schema[idx].Type == nil {
			// another group
			child := &group{}
			idx, err = child.create(schema, name, flatMap, idx, dLevel, rLevel)
			if err != nil {
				return 0, err
			}
			g.children = append(g.children, child)
		} else {
			child := &primitive{}
			idx, err = child.create(schema, name, flatMap, idx, dLevel, rLevel)
			if err != nil {
				return 0, err
			}
			g.children = append(g.children, child)
			flatMap[child.flatName] = child
		}
	}

	return idx, nil
}

func (p *primitive) create(schema []*parquet.SchemaElement, name string, flatMap map[string]Column, idx, dLevel, rLevel int) (int, error) {
	s := schema[idx]

	// TODO: validate Name is not empty
	if s.RepetitionType == nil {
		return 0, errors.Errorf("field RepetitionType is nil in index %d", idx)
	}

	// TODO : validate here
	if *s.RepetitionType != parquet.FieldRepetitionType_REQUIRED {
		dLevel++
	}

	if *s.RepetitionType == parquet.FieldRepetitionType_REPEATED {
		rLevel++
	}

	p.SchemaElement = s
	p.rLevel = rLevel
	p.dLevel = dLevel
	p.flatName = name + "." + s.Name
	return idx, nil
}

func MakeSchema(meta *parquet.FileMetaData) (*Schema, error) {
	s := &Schema{}
	s.flatMap = make(map[string]Column)
	end, err := s.root.create(meta.Schema, "", s.flatMap, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	if end != len(meta.Schema)-1 {
		return s, fmt.Errorf("too many SchemaElements, only %d out of %d have been used",
			end, len(meta.Schema))
	}

	return s, nil
}