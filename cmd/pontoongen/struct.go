package main

import (
	"go/ast"
	"go/token"
	"go/types"
	"reflect"

	"github.com/pkg/errors"
	"golang.org/x/tools/go/ast/astutil"
)

func (b *builder) getTypeDescCached(tt types.Type) (*typeDesc, error) {
	if r, ok := typeCache[tt]; ok {
		return r, nil
	}
	ret, err := b.getTypeDesc(tt)
	typeCache[tt] = ret
	return ret, err
}

func (b *builder) getTypeDesc(tt types.Type) (*typeDesc, error) {

	switch t := tt.(type) {
	case *types.Basic:
		return &typeDesc{isScalar: true, name: t.Name()}, nil
	case *types.Chan:
		return nil, nil
	case *types.Slice:
		ut, err := b.getTypeDescCached(t.Elem())
		if err != nil {
			return nil, errors.Wrapf(err, "creating typedesc for '%v'", t.String())
		}
		return &typeDesc{
			name: t.String(),
			isSlice: &descSlice{
				t: ut,
			},
		}, nil
	case *types.Map:
		key, err := b.getTypeDescCached(t.Key())
		if err != nil {
			return nil, errors.Wrapf(err, "parsing key type of map '%v'", t.String())
		}
		value, err := b.getTypeDescCached(t.Elem())
		if err != nil {
			return nil, errors.Wrapf(err, "parsing value type of map '%v'", t.String())
		}
		return &typeDesc{
			name: t.String(),
			isMap: &descMap{
				key:   key,
				value: value,
			}}, nil
	case *types.Pointer:
		ut, err := b.getTypeDescCached(t.Elem())
		if err != nil {
			return nil, errors.Wrapf(err, "parsing ptr value of '%v'", t.String())
		}
		return &typeDesc{
			name:  t.String(),
			isPtr: ut,
		}, nil
	case *types.Named:
		switch tu := t.Underlying().(type) {
		case *types.Basic:
			return b.getTypeDescCached(tu)
		case *types.Struct:
		default:
			return nil, errors.Errorf("unknown underlying type '%v' of Named '%v' (value '%v')", reflect.TypeOf(tu).String(), reflect.TypeOf(tt).String(), tt.String())
		}
	default:
		return nil, errors.Errorf("unknown Type of '%v' (value '%v')", reflect.TypeOf(tt).String(), tt.String())
	}
	t := tt.(*types.Named)
	docs, err := b.getStructDocs(t.Obj().Pos())
	if err != nil {
		return nil, errors.Wrap(err, "when extracting struct docs")
	}

	st := t.Underlying().(*types.Struct)

	ret := typeDesc{
		name:     t.Obj().Pkg().Name() + "." + t.Obj().Name(),
		doc:      docs.Doc,
		isStruct: &descStruct{},
	}

	if ret.name == "time.Time" {
		ret.isStruct = nil
		ret.isSpecial = specialTypeTime
		return &ret, nil
	}

	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)

		if f.Embedded() {
			etd, err := b.getTypeDescCached(f.Type())
			if err != nil {
				return nil, errors.Wrapf(err, "processing embedded field type '%v'", f.Name())
			}
			ret.isStruct.embeds = append(ret.isStruct.embeds, etd)
			continue
		}

		fd := descField{
			name: f.Name(),
			doc:  docs.DocsByFields[f.Name()],
			tags: docs.TagsByFields[f.Name()],
		}

		ft, err := b.getTypeDescCached(f.Type())
		if err != nil {
			return nil, errors.Wrapf(err, "parsing field '%v'", f.Name())
		}
		fd.t = ft

		ret.isStruct.fields = append(ret.isStruct.fields, fd)
	}
	return &ret, nil
}

func (b *builder) getStructDocs(pos token.Pos) (*structDoc, error) {
	f, err := b.astFindFile(pos)
	if err != nil {
		return &structDoc{}, nil
		// TODO load files from imported packages for comments
		//return nil, errors.Wrap(err, "astFindFile failed")
	}
	reg, _ := astutil.PathEnclosingInterval(f, pos-1, pos)

	anode := reg[0].(*ast.GenDecl)

	desc := structDoc{
		DocsByFields: map[string]string{},
		TagsByFields: map[string]string{},
	}

	if anode.Doc != nil {
		desc.Doc = anode.Doc.Text()
	}

	if len(anode.Specs) != 1 {
		return nil, errors.Errorf("Specs length is '%v', expected 1", len(anode.Specs))
	}
	spec := anode.Specs[0].(*ast.TypeSpec)

	stype := spec.Type.(*ast.StructType)

	for _, f := range stype.Fields.List {
		if len(f.Names) == 0 {
			continue // embedded struct
		}
		if f.Tag != nil {
			desc.TagsByFields[f.Names[0].Name] = f.Tag.Value
		}
		if f.Doc != nil {
			desc.DocsByFields[f.Names[0].Name] = f.Doc.Text()
		}
	}

	return &desc, nil
}

type structDoc struct {
	Doc          string
	DocsByFields map[string]string
	TagsByFields map[string]string
}
