import type Konva from 'konva'

import type { StageObject } from '../shared/stage-types'

export const syncStageObjectHierarchy = (
  objects: Record<string, StageObject>,
  objectNodes: Map<string, Konva.Group>,
  objectRoot: Konva.Group,
) => {
  const attachChildren = (parentId: string | null) => {
    const parent = parentId ? objectNodes.get(parentId) : objectRoot
    if (!parent) return
    const children = Object.values(objects)
      .filter((object) => (object.parentId && objects[object.parentId] ? object.parentId : null) === parentId)
      .sort((a, b) => a.transform.z - b.transform.z || a.transform.order - b.transform.order)
    const contentCount = parentId
      ? Array.from(parent.getChildren()).filter((child) => !(child as Konva.Node).getAttr('stageObjectId')).length
      : 0
    children.forEach((object, index) => {
      const node = objectNodes.get(object.id)
      if (!node) return
      if (node.getParent() !== parent) node.moveTo(parent)
      node.zIndex(contentCount + index)
      attachChildren(object.id)
    })
  }

  attachChildren(null)
}
