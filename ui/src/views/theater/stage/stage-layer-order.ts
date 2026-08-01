import type { StageObject } from '../shared/stage-types'

const finiteLayerValue = (value: number) => Number.isFinite(value) ? value : 0
const compareObjectIds = (left: string, right: string) => left < right ? -1 : left > right ? 1 : 0

export const compareStageLayersBottomToTop = (left: StageObject, right: StageObject) => (
  finiteLayerValue(left.transform.z) - finiteLayerValue(right.transform.z)
  || finiteLayerValue(left.transform.order) - finiteLayerValue(right.transform.order)
  || compareObjectIds(left.id, right.id)
)

export const compareStageLayersTopToBottom = (left: StageObject, right: StageObject) => (
  -compareStageLayersBottomToTop(left, right)
)

export interface StageLayerRank {
  z: number
  order: number
}

export const stageLayerRankBetween = (
  above: StageObject | undefined,
  below: StageObject | undefined,
): StageLayerRank | null => {
  if (above && below) {
    if (above.transform.z > below.transform.z) {
      const z = (above.transform.z + below.transform.z) / 2
      if (Number.isFinite(z) && z !== above.transform.z && z !== below.transform.z) return { z, order: z }
    } else if (above.transform.z === below.transform.z && above.transform.order > below.transform.order) {
      const order = (above.transform.order + below.transform.order) / 2
      if (Number.isFinite(order) && order !== above.transform.order && order !== below.transform.order) {
        return { z: above.transform.z, order }
      }
    }
    return null
  }

  const z = above ? above.transform.z - 1 : below ? below.transform.z + 1 : 1
  return Number.isFinite(z) ? { z, order: z } : null
}
