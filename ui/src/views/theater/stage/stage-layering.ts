import type Konva from 'konva'

import type { StageObject } from '../shared/stage-types'

export const syncStageObjectHierarchy = (
  objects: Record<string, StageObject>,
  objectNodes: Map<string, Konva.Group>,
  objectRoot: Konva.Group,
) => {
  const childrenByParent = new Map<string | null, StageObject[]>()
  Object.values(objects).forEach((object) => {
    const parentId = object.parentId && objects[object.parentId] ? object.parentId : null
    const children = childrenByParent.get(parentId) || []
    children.push(object)
    childrenByParent.set(parentId, children)
  })
  childrenByParent.forEach((children) => {
    children.sort((left, right) => left.transform.z - right.transform.z || left.transform.order - right.transform.order)
  })
  const attachChildren = (parentId: string | null) => {
    const parent = parentId ? objectNodes.get(parentId) : objectRoot
    if (!parent) return
    const children = childrenByParent.get(parentId) || []
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
