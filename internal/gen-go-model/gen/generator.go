package gen

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/dave/jennifer/jen"

	"github.com/thanhpd56/dbml-go/core"
	"github.com/thanhpd56/dbml-go/internal/gen-go-model/genutil"
)

type generator struct {
	dbml             *core.DBML
	out              string
	gopackage        string
	fieldtags        []string
	types            map[string]jen.Code
	shouldGenTblName bool
}

func newgen() *generator {
	return &generator{
		types: make(map[string]jen.Code),
	}
}

func (g *generator) reset(rememberAlias bool) {
	g.dbml = nil
	if !rememberAlias {
		g.types = make(map[string]jen.Code)
	}
}

func (g *generator) file() *jen.File {
	return jen.NewFilePathName(g.out, g.gopackage)
}

func (g *generator) generate() error {
	if err := g.genEnums(); err != nil {
		return err
	}
	return nil
}

func (g *generator) genEnums() error {
	for _, enum := range g.dbml.Enums {
		if err := g.genEnum(enum); err != nil {
			return err
		}
	}

	toColumnNameToRelationships, fromColumnNameToRelationships := g.getFullRelationShips()

	for _, table := range g.dbml.Tables {
		if err := g.genTable(table, toColumnNameToRelationships, fromColumnNameToRelationships); err != nil {
			return err
		}
	}

	return nil
}

func (g *generator) genEnum(enum core.Enum) error {
	f := jen.NewFilePathName(g.out, g.gopackage)

	enumOriginName := genutil.NormalizeTypeName(enum.Name)
	enumGoTypeName := genutil.NormalizeGoTypeName(enum.Name)

	f.PackageComment("Code generated by dbml-gen-go-model. DO NOT EDIT.")
	f.PackageComment("Supported by thanhpd56@2020")
	f.Commentf("%s is generated type for enum '%s'", enumGoTypeName, enumOriginName)
	f.Type().Id(enumGoTypeName).String()

	f.Const().DefsFunc(func(group *jen.Group) {
		for _, value := range enum.Values {
			enumName := fmt.Sprintf("%s_%s", enumOriginName, strings.ToLower(value.Name))
			enumName = genutil.NormalizeGoTypeName(enumName)
			v := group.Id(enumName).Op("=").Id(enumGoTypeName).Parens(jen.Lit(value.Name))
			if value.Note != "" {
				v.Comment(value.Note)
			}
		}
	})

	g.types[enum.Name] = jen.Id(enumGoTypeName)

	return f.Save(fmt.Sprintf("%s/%s.enum.go", g.out, genutil.Normalize(enum.Name)))
}

func (g *generator) genTable(table core.Table, toColumnNameToRelationships map[string][]core.Relationship, fromColumnNameToRelationships map[string][]core.Relationship) error {
	f := jen.NewFilePathName(g.out, g.gopackage)

	tableOriginName := genutil.Normalize(table.Name)
	tableGoTypeName := genutil.NormalizeGoTypeName(table.Name)

	f.PackageComment("Code generated by dbml-gen-go-model. DO NOT EDIT.")
	f.PackageComment("Supported by thanhpd56@2020")
	f.Commentf("%s is generated type for table '%s'", tableGoTypeName, tableOriginName)

	var genColumnErr error

	cols := make([]string, 0)

	f.Type().Id(tableGoTypeName).StructFunc(func(group *jen.Group) {
		for _, column := range table.Columns {

			columnName := genutil.NormalLizeGoName(column.Name)
			columnOriginName := genutil.Normalize(column.Name)
			t, ok := g.getJenType(column.Type)
			if !ok {
				genColumnErr = fmt.Errorf("type '%s' is not support", column.Type)
			}
			if column.Settings.Note != "" {
				group.Comment(column.Settings.Note)
			}

			gotags := make(map[string]string)
			for _, t := range g.fieldtags {
				gotags[strings.TrimSpace(t)] = columnOriginName
			}
			if column.Settings.Null && column.Settings.Default == "" {
				group.Id(columnName).Add(jen.Op("*")).Add(t).Tag(gotags)
			} else {
				group.Id(columnName).Add(t).Tag(gotags)
			}
			cols = append(cols, columnOriginName)

			// users.department_id > departments.id
			fullColumnID := fmt.Sprintf("%s.%s", table.Name, column.Name)
			// [{from: }]
			toRelationships := toColumnNameToRelationships[fullColumnID]

			for _, relationship := range toRelationships {
				// split department_id => to get department
				relationName := strings.Split(column.Name, "_id")[0]
				// split departments.id => departments
				fromTypeName := strings.Split(relationship.From, ".")[0]
				// normalize to Department
				fromGoTypeName := genutil.NormalizeGoTypeName(fromTypeName)

				// split departments.id => departments
				toTypeName := strings.Split(relationship.To, ".")[0]
				// normalize to Department
				toGoTypeName := genutil.NormalizeGoTypeName(toTypeName)

				// normalize relationName department to Department
				fieldName := genutil.NormalizeGoTypeName(relationName)
				tags := map[string]string{"gorm": fmt.Sprintf("foreignkey:%s", column.Name)}

				if relationship.Type == core.OneToMany {
					group.Id(fieldName).Id(fromGoTypeName).Tag(tags)
				} else if relationship.Type == core.OneToOne {
					group.Id(fieldName).Op("*").Id(fromGoTypeName).Tag(tags)
				} else if relationship.Type == core.ManyToOne {
					group.Id(fieldName).Op("*").Id(toGoTypeName).Tag(tags)
				}
			}
			// departments.id > users.department_id
			fromRelationships := fromColumnNameToRelationships[fullColumnID]

			for _, relationship := range fromRelationships {
				// users.id => users
				toTypeName := strings.Split(relationship.To, ".")[0]
				toColumnName := strings.Split(relationship.To, ".")[1]
				// users => User
				toGoTypeName := genutil.NormalizeGoTypeName(toTypeName)

				// users.id => users
				fromTypeName := strings.Split(relationship.From, ".")[0]
				// users => User
				fromGoTypeName := genutil.NormalizeGoTypeName(fromTypeName)
				tags := map[string]string{"gorm": fmt.Sprintf("foreignkey:%s", toColumnName)}

				if relationship.Type == core.OneToMany {
					// Users
					name := genutil.GoInitialismCamelCase(toTypeName)
					// Users []User
					group.Id(name).Index().Id(toGoTypeName).Tag(tags)

				} else if relationship.Type == core.OneToOne {
					// User
					name := genutil.NormalizeGoTypeName(toTypeName)
					// User *User
					group.Id(name).Op("*").Id(toGoTypeName).Tag(tags)
				} else if relationship.Type == core.ManyToOne {
					// Users
					name := genutil.GoInitialismCamelCase(fromTypeName)
					// Users []User
					group.Id(name).Index().Id(fromGoTypeName).Tag(tags)
				}
			}
		}
	})

	if genColumnErr != nil {
		return genColumnErr
	}

	tableMetadataType := "__tbl_" + tableOriginName
	tableMetadataColumnsType := tableMetadataType + "_columns"

	f.Commentf("// table '%s' columns list struct", tableOriginName)
	f.Type().Id(tableMetadataColumnsType).StructFunc(func(group *jen.Group) {
		for _, column := range table.Columns {
			name := genutil.NormalLizeGoName(column.Name)
			group.Id(name).String()
		}
	})

	f.Commentf("// table '%s' metadata struct", tableOriginName)
	f.Type().Id("__tbl_"+tableOriginName).Struct(
		jen.Id("Name").String(),
		jen.Id("Columns").Id(tableMetadataColumnsType),
	)

	tableMetadataVar := "_tbl_" + tableOriginName

	f.Commentf("// table '%s' metadata info", tableOriginName)
	f.Var().Id(tableMetadataVar).Op("=").Id(tableMetadataType).Values(jen.DictFunc(func(d jen.Dict) {
		d[jen.Id("Name")] = jen.Lit(tableOriginName)
		d[jen.Id("Columns")] = jen.Id(tableMetadataColumnsType).Values(jen.DictFunc(func(d jen.Dict) {
			for _, column := range table.Columns {
				columnName := genutil.NormalLizeGoName(column.Name)
				columnOriginName := genutil.Normalize(column.Name)
				d[jen.Id(columnName)] = jen.Lit(columnOriginName)
			}
		}))
	}))

	f.Commentf("GetColumns return list columns name for table '%s'", tableOriginName)
	f.Func().Params(
		jen.Op("*").Id(tableMetadataType),
	).Id("GetColumns").Params().Index().String().Block(
		jen.Return(jen.Index().String().ValuesFunc(func(g *jen.Group) {
			for _, col := range cols {
				g.Lit(col)
			}
		})),
	)

	f.Commentf("T return metadata info for table '%s'", tableOriginName)
	f.Func().Params(
		jen.Op("*").Id(tableGoTypeName),
	).Id("T").Params().Op("*").Id(tableMetadataType).Block(
		jen.Return().Op("&").Id(tableMetadataVar),
	)

	if g.shouldGenTblName {
		f.Commentf("TableName return table name")
		f.Func().Params(
			jen.Id(tableGoTypeName),
		).Id("TableName").Params().Id("string").Block(
			jen.Return(jen.Lit(tableOriginName)),
		)
	}

	return f.Save(fmt.Sprintf("%s/%s.table.go", g.out, genutil.Normalize(table.Name)))
}

const primeTypePattern = `^(\w+)(\(d+\))?`

var (
	regexType    = regexp.MustCompile(primeTypePattern)
	builtinTypes = map[string]jen.Code{
		"int":       jen.Int(),
		"int8":      jen.Int8(),
		"int16":     jen.Int16(),
		"int32":     jen.Int32(),
		"int64":     jen.Int64(),
		"bigint":    jen.Int64(),
		"uint":      jen.Uint(),
		"uint8":     jen.Uint8(),
		"uint16":    jen.Uint16(),
		"uint32":    jen.Uint32(),
		"uint64":    jen.Uint64(),
		"float":     jen.Float64(),
		"float32":   jen.Float32(),
		"float64":   jen.Float64(),
		"bool":      jen.Bool(),
		"text":      jen.String(),
		"varchar":   jen.String(),
		"char":      jen.String(),
		"byte":      jen.Byte(),
		"rune":      jen.Rune(),
		"timestamp": jen.Int(),
		"json":      jen.Qual("gorm.io/datatypes", "JSON"),
		"datetime":  jen.Qual("time", "Time"),
	}
)

func (g *generator) getJenType(s string) (jen.Code, bool) {
	m := regexType.FindStringSubmatch(s)
	if len(m) >= 2 {
		// lookup for builtin type
		if t, ok := builtinTypes[m[1]]; ok {
			return t, ok
		}
	}
	t, ok := g.types[s]
	return t, ok
}

func (g *generator) getFullRelationShips() (toColumnNameToRelationships map[string][]core.Relationship, fromColumnNameToRelationships map[string][]core.Relationship) {
	toColumnNameToRelationships = map[string][]core.Relationship{}
	fromColumnNameToRelationships = map[string][]core.Relationship{}
	for _, ref := range g.dbml.Refs {
		for _, relationship := range ref.Relationships {
			toColumnNameToRelationships[relationship.To] = append(toColumnNameToRelationships[relationship.To], relationship)
			fromColumnNameToRelationships[relationship.From] = append(fromColumnNameToRelationships[relationship.From], relationship)
		}
	}
	for _, table := range g.dbml.Tables {
		// inline relationships
		for _, column := range table.Columns {
			// support inline relationship
			fullColumnID := fmt.Sprintf("%s.%s", table.Name, column.Name)
			toRelationships := toColumnNameToRelationships[fullColumnID]
			fromRelationships := fromColumnNameToRelationships[fullColumnID]

			if column.Settings.Ref.To != "" {
				toRelationships = append(toRelationships, core.Relationship{
					From: fullColumnID,
					To:   column.Settings.Ref.To,
					Type: column.Settings.Ref.Type,
				})

				var reverseRefType core.RelationshipType = core.OneToOne
				if column.Settings.Ref.Type == core.OneToMany {
					reverseRefType = core.ManyToOne
				}
				if column.Settings.Ref.Type == core.ManyToOne {
					reverseRefType = core.OneToMany
				}
				fromRelationships = append(fromRelationships, core.Relationship{
					From: column.Settings.Ref.To,
					To:   fullColumnID,
					Type: reverseRefType,
				})

				toColumnNameToRelationships[fullColumnID] = toRelationships
				fromColumnNameToRelationships[column.Settings.Ref.To] = fromRelationships
			}
		}
	}

	return
}
