package service

import (
	"math"
	"sort"
)

type theaterLayoutBounds struct{ left, right, top, bottom float64 }

// Bounds are in the common parent's coordinates. Parent transforms are shared
// by every participant and are deliberately neither inverted nor overwritten.
func theaterObjectAABB(object TheaterObjectSnapshot) theaterLayoutBounds {
	angle := object.Rotation * math.Pi / 180
	scaleX, scaleY := object.ScaleX, object.ScaleY
	if scaleX <= 0 {
		scaleX = object.Scale
	}
	if scaleY <= 0 {
		scaleY = object.Scale
	}
	if scaleX <= 0 {
		scaleX = 1
	}
	if scaleY <= 0 {
		scaleY = 1
	}
	w, h := object.Width*scaleX, object.Height*scaleY
	halfW := (math.Abs(math.Cos(angle))*w + math.Abs(math.Sin(angle))*h) / 2
	halfH := (math.Abs(math.Sin(angle))*w + math.Abs(math.Cos(angle))*h) / 2
	return theaterLayoutBounds{object.X - halfW, object.X + halfW, object.Y - halfH, object.Y + halfH}
}

func theaterLayoutObjects(snapshot TheaterSharedSnapshot, ids []string) ([]TheaterObjectSnapshot, error) {
	if len(ids) < 1 || len(ids) > theaterMaxBatchUpdates {
		return nil, theaterPayloadError("layout objectIds 必须为 1..200 项")
	}
	objects, seen := make([]TheaterObjectSnapshot, 0, len(ids)), map[string]bool{}
	for _, id := range ids {
		object, found := theaterDesignFindObject(snapshot, id)
		if !found {
			return nil, newTheaterError(TheaterErrorNotFound, "layout 对象不存在", 404, map[string]any{"objectId": id})
		}
		if seen[id] {
			return nil, theaterPayloadError("layout objectIds 不能重复")
		}
		seen[id] = true
		if object.Kind == "effect" {
			return nil, theaterPayloadError("effect 使用独立设计坐标，不能参加普通 layout")
		}
		if err := theaterDesignObjectUnlocked(snapshot, object); err != nil {
			return nil, err
		}
		if len(objects) > 0 && (derefString(object.SceneID) != derefString(objects[0].SceneID) || derefString(object.ParentID) != derefString(objects[0].ParentID)) {
			return nil, theaterPayloadError("layout 对象必须同 scene/persistent scope、同 parentId")
		}
		objects = append(objects, object)
	}
	return objects, nil
}

func planTheaterLayout(snapshot TheaterSharedSnapshot, step any) (*theaterObjectBatchUpdatePayload, []string, error) {
	var ids []string
	switch s := step.(type) {
	case *TheaterDesignLayoutAlign:
		ids = s.ObjectIDs
	case *TheaterDesignLayoutDistribute:
		ids = s.ObjectIDs
	case *TheaterDesignLayoutGrid:
		ids = s.ObjectIDs
	}
	objects, err := theaterLayoutObjects(snapshot, ids)
	if err != nil {
		return nil, ids, err
	}
	bounds := theaterObjectAABB(objects[0])
	for _, object := range objects[1:] {
		b := theaterObjectAABB(object)
		bounds.left, bounds.right = math.Min(bounds.left, b.left), math.Max(bounds.right, b.right)
		bounds.top, bounds.bottom = math.Min(bounds.top, b.top), math.Max(bounds.bottom, b.bottom)
	}
	switch s := step.(type) {
	case *TheaterDesignLayoutAlign:
		for index := range objects {
			o := &objects[index]
			b := theaterObjectAABB(*o)
			switch s.Mode {
			case "left":
				o.X += bounds.left - b.left
			case "center_x":
				o.X = (bounds.left + bounds.right) / 2
			case "right":
				o.X += bounds.right - b.right
			case "top":
				o.Y += bounds.top - b.top
			case "center_y":
				o.Y = (bounds.top + bounds.bottom) / 2
			case "bottom":
				o.Y += bounds.bottom - b.bottom
			default:
				return nil, ids, theaterPayloadError("layout.align mode 无效")
			}
		}
	case *TheaterDesignLayoutDistribute:
		if len(objects) < 2 {
			return nil, ids, theaterPayloadError("layout.distribute 至少需要两个对象")
		}
		if s.Mode != "horizontal" && s.Mode != "vertical" {
			return nil, ids, theaterPayloadError("layout.distribute mode 无效")
		}
		horizontal := s.Mode == "horizontal"
		center := func(o TheaterObjectSnapshot) float64 {
			if horizontal {
				return o.X
			}
			return o.Y
		}
		edges := func(o TheaterObjectSnapshot) (float64, float64) {
			b := theaterObjectAABB(o)
			if horizontal {
				return b.left, b.right
			}
			return b.top, b.bottom
		}
		sort.Slice(objects, func(i, j int) bool {
			if center(objects[i]) == center(objects[j]) {
				return objects[i].ID < objects[j].ID
			}
			return center(objects[i]) < center(objects[j])
		})
		first, _ := edges(objects[0])
		_, last := edges(objects[len(objects)-1])
		total := 0.0
		for _, o := range objects {
			a, b := edges(o)
			total += b - a
		}
		gap := (last - first - total) / float64(len(objects)-1)
		cursor := first
		for i := range objects {
			a, b := edges(objects[i])
			delta := cursor - a
			if horizontal {
				objects[i].X += delta
			} else {
				objects[i].Y += delta
			}
			cursor += b - a + gap
		}
	case *TheaterDesignLayoutGrid:
		if s.Columns < 1 || s.Columns > len(objects) || !theaterFinite(s.GapX) || !theaterFinite(s.GapY) || s.GapX < 0 || s.GapY < 0 {
			return nil, ids, theaterPayloadError("layout.grid columns/gaps 无效")
		}
		x, y := bounds.left, bounds.top
		if s.OriginX != nil {
			x = *s.OriginX
		}
		if s.OriginY != nil {
			y = *s.OriginY
		}
		if !theaterFinite(x) || !theaterFinite(y) {
			return nil, ids, theaterPayloadError("layout.grid origin 无效")
		}
		widths, heights := make([]float64, s.Columns), make([]float64, (len(objects)+s.Columns-1)/s.Columns)
		for i, o := range objects {
			b := theaterObjectAABB(o)
			widths[i%s.Columns] = math.Max(widths[i%s.Columns], b.right-b.left)
			heights[i/s.Columns] = math.Max(heights[i/s.Columns], b.bottom-b.top)
		}
		columnX, rowY := make([]float64, len(widths)), make([]float64, len(heights))
		for i, w := range widths {
			columnX[i] = x + w/2
			x += w + s.GapX
		}
		for i, h := range heights {
			rowY[i] = y + h/2
			y += h + s.GapY
		}
		for i := range objects {
			objects[i].X = columnX[i%s.Columns]
			objects[i].Y = rowY[i/s.Columns]
		}
	}
	updates := &theaterObjectBatchUpdatePayload{}
	for _, o := range objects {
		if !theaterFinite(o.X) || !theaterFinite(o.Y) {
			return nil, ids, theaterPayloadError("layout transform 溢出")
		}
		updates.Updates = append(updates.Updates, theaterObjectUpdatePayload{ObjectID: o.ID, Fields: map[string]any{"x": o.X, "y": o.Y}})
	}
	return updates, ids, nil
}

func theaterFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
