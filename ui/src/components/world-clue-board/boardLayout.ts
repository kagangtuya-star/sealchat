import type { WorldClueSummary } from '@/stores/worldClue'
import type { WorldClueBoardPlacement, WorldClueBoardRelation } from '@/stores/worldClueBoard'
import type { BoardNodeLayout } from './boardTypes'
import { forceCenter, forceCollide, forceLink, forceManyBody, forceSimulation, type SimulationNodeDatum } from 'd3-force'

export const BOARD_NODE_WIDTH = 240
export const BOARD_NODE_HEIGHT = 196
export const BOARD_GRID_GAP = 48

interface PlacementRect {
  x: number
  y: number
  width: number
  height: number
}

interface GridCursor {
  diagonal: number
  row: number
}

const BOARD_GRID_ORIGIN_X = 72
const BOARD_GRID_ORIGIN_Y = 72

function placementRect(placement: WorldClueBoardPlacement): PlacementRect {
  const width = placement.width
  return {
    x: Number.isFinite(placement.x) ? placement.x : 0,
    y: Number.isFinite(placement.y) ? placement.y : 0,
    width: typeof width === 'number' && Number.isFinite(width) && width > 0 ? width : BOARD_NODE_WIDTH,
    height: BOARD_NODE_HEIGHT,
  }
}

function gridKey(column: number, row: number) {
  return `${column}:${row}`
}

function markPlacementOccupied(occupied: Set<string>, placement: WorldClueBoardPlacement) {
  const rect = placementRect(placement)
  const stepX = BOARD_NODE_WIDTH + BOARD_GRID_GAP
  const stepY = BOARD_NODE_HEIGHT + BOARD_GRID_GAP
  const minColumn = Math.max(0, Math.floor((rect.x - BOARD_GRID_ORIGIN_X - BOARD_NODE_WIDTH) / stepX) + 1)
  const maxColumn = Math.ceil((rect.x + rect.width - BOARD_GRID_ORIGIN_X) / stepX) - 1
  const minRow = Math.max(0, Math.floor((rect.y - BOARD_GRID_ORIGIN_Y - BOARD_NODE_HEIGHT) / stepY) + 1)
  const maxRow = Math.ceil((rect.y + rect.height - BOARD_GRID_ORIGIN_Y) / stepY) - 1
  if (maxColumn < minColumn || maxRow < minRow) return
  for (let column = minColumn; column <= maxColumn; column += 1) {
    for (let row = minRow; row <= maxRow; row += 1) occupied.add(gridKey(column, row))
  }
}

function nextTemporaryPlacement(cursor: GridCursor, occupied: Set<string>): WorldClueBoardPlacement {
  const stepX = BOARD_NODE_WIDTH + BOARD_GRID_GAP
  const stepY = BOARD_NODE_HEIGHT + BOARD_GRID_GAP
  for (;;) {
    const row = cursor.row
    const column = cursor.diagonal - row
    cursor.row += 1
    if (cursor.row > cursor.diagonal) {
      cursor.diagonal += 1
      cursor.row = 0
    }
    if (!occupied.has(gridKey(column, row))) return { x: BOARD_GRID_ORIGIN_X + column * stepX, y: BOARD_GRID_ORIGIN_Y + row * stepY }
  }
}

export function buildNodeLayouts(
  summaries: WorldClueSummary[],
  placements: Record<string, WorldClueBoardPlacement>,
  temporaryPlacementMap: Map<string, WorldClueBoardPlacement> = new Map(),
  knownSummaryIds?: ReadonlySet<string>,
): BoardNodeLayout[] {
  const occupied = new Set<string>()
  const summaryIds = knownSummaryIds ?? new Set(summaries.map(summary => summary.id))
  for (const placement of Object.values(placements)) markPlacementOccupied(occupied, placement)
  for (const [id, placement] of temporaryPlacementMap) {
    if (placements[id] || !summaryIds.has(id)) {
      temporaryPlacementMap.delete(id)
      continue
    }
    markPlacementOccupied(occupied, placement)
  }

  const cursor: GridCursor = { diagonal: 0, row: 0 }
  return summaries.map(summary => {
    const placement = placements[summary.id]
    const temporary = !placement
    let fallback: WorldClueBoardPlacement
    if (placement) {
      temporaryPlacementMap.delete(summary.id)
      fallback = placement
    } else {
      const temporaryPlacement = temporaryPlacementMap.get(summary.id)
      if (temporaryPlacement) {
        fallback = temporaryPlacement
      } else {
        fallback = nextTemporaryPlacement(cursor, occupied)
        temporaryPlacementMap.set(summary.id, fallback)
        markPlacementOccupied(occupied, fallback)
      }
    }
    return {
      id: summary.id,
      summary,
      x: fallback.x,
      y: fallback.y,
      width: Math.max(180, Math.min(480, Number(fallback.width) || BOARD_NODE_WIDTH)),
      height: BOARD_NODE_HEIGHT,
      pinned: fallback.pinned === true,
      temporary,
    }
  })
}

interface LayoutSimulationNode extends SimulationNodeDatum {
  id: string
  x: number
  y: number
  fx?: number | null
  fy?: number | null
  width: number
  height: number
  pinned: boolean
}

interface LayoutSimulationLink {
  source: string
  target: string
}

function separateOverlappingTargets(nodes: LayoutSimulationNode[], targetIds: Set<string>) {
  const maxPasses = Math.min(64, Math.max(1, nodes.length * 2))
  for (let pass = 0; pass < maxPasses; pass += 1) {
    let changed = false
    for (let firstIndex = 0; firstIndex < nodes.length; firstIndex += 1) {
      for (let secondIndex = firstIndex + 1; secondIndex < nodes.length; secondIndex += 1) {
        const first = nodes[firstIndex]
        const second = nodes[secondIndex]
        const firstTarget = targetIds.has(first.id)
        const secondTarget = targetIds.has(second.id)
        const firstMovable = firstTarget && first.fx == null && first.fy == null
        const secondMovable = secondTarget && second.fx == null && second.fy == null
        if ((!firstTarget && !secondTarget) || (!firstMovable && !secondMovable)) continue
        const overlapX = Math.min(first.x + first.width, second.x + second.width) - Math.max(first.x, second.x)
        const overlapY = Math.min(first.y + first.height, second.y + second.height) - Math.max(first.y, second.y)
        if (overlapX <= 0 || overlapY <= 0) continue

        const moving = secondMovable ? second : first
        const obstacle = moving === first ? second : first
        const movingCenterX = moving.x + moving.width / 2
        const obstacleCenterX = obstacle.x + obstacle.width / 2
        const movingCenterY = moving.y + moving.height / 2
        const obstacleCenterY = obstacle.y + obstacle.height / 2
        if (overlapX <= overlapY) {
          const direction = movingCenterX < obstacleCenterX || (movingCenterX === obstacleCenterX && moving.id > obstacle.id) ? -1 : 1
          moving.x += direction * (overlapX + 1)
        } else {
          const direction = movingCenterY < obstacleCenterY || (movingCenterY === obstacleCenterY && moving.id > obstacle.id) ? -1 : 1
          moving.y += direction * (overlapY + 1)
        }
        changed = true
      }
    }
    if (!changed) break
  }
}

/**
 * A bounded, user-triggered arrangement. The simulation only receives cloned
 * nodes, so it cannot mutate the board document while force ticks are running.
 * All visible nodes participate as either movable targets or fixed obstacles.
 */
export function arrangeLayouts(
  nodes: BoardNodeLayout[],
  targetIds?: Set<string>,
  relations: WorldClueBoardRelation[] = [],
): Map<string, WorldClueBoardPlacement> {
  const targetIdsSet = targetIds ? new Set(targetIds) : new Set(nodes.map(node => node.id))
  const targets = nodes.filter(node => targetIdsSet.has(node.id))
  if (!targets.length) return new Map()

  const columns = Math.max(1, Math.ceil(Math.sqrt(nodes.length)))
  const simulationNodes: LayoutSimulationNode[] = nodes.map((node, index) => {
    const column = index % columns
    const row = Math.floor(index / columns)
    const next: LayoutSimulationNode = {
      id: node.id,
      x: Number.isFinite(node.x) ? node.x : 80 + column * (BOARD_NODE_WIDTH + BOARD_GRID_GAP),
      y: Number.isFinite(node.y) ? node.y : 80 + row * (BOARD_NODE_HEIGHT + BOARD_GRID_GAP),
      width: node.width,
      height: node.height,
      pinned: node.pinned,
    }
    if (node.pinned || !targetIdsSet.has(node.id)) {
      next.fx = next.x
      next.fy = next.y
    }
    return next
  })
  const simulationIds = new Set(simulationNodes.map(node => node.id))
  const links: LayoutSimulationLink[] = relations
    .filter(relation => relation.sourceRef.kind === 'clue' && relation.targetRef.kind === 'clue' && simulationIds.has(relation.sourceRef.id) && simulationIds.has(relation.targetRef.id))
    .map(relation => ({ source: relation.sourceRef.id, target: relation.targetRef.id }))

  const centerX = simulationNodes.reduce((sum, node) => sum + (node.x || 0), 0) / simulationNodes.length
  const centerY = simulationNodes.reduce((sum, node) => sum + (node.y || 0), 0) / simulationNodes.length
  const simulation = forceSimulation<LayoutSimulationNode>(simulationNodes)
    .force('charge', forceManyBody<LayoutSimulationNode>().strength(-260).distanceMax(1200))
    .force('collide', forceCollide<LayoutSimulationNode>(node => Math.max(node.width, node.height) / 2 + BOARD_GRID_GAP / 2).iterations(2))
    .force('center', forceCenter(centerX, centerY))
  // A light relation force improves readability without turning the board into
  // a continuously animated graph. The fixed-tick run below is deterministic
  // for the same input and is stopped before the result is committed.
  simulation.force('links', forceLink<LayoutSimulationNode, LayoutSimulationLink>(links)
    .id(node => node.id)
    .distance(BOARD_NODE_WIDTH + BOARD_GRID_GAP)
    .strength(0.18))
  simulation.stop()
  for (let tick = 0; tick < 72; tick += 1) simulation.tick()
  simulation.stop()
  separateOverlappingTargets(simulationNodes, targetIdsSet)

  const result = new Map<string, WorldClueBoardPlacement>()
  const nodeById = new Map(nodes.map(node => [node.id, node]))
  for (const node of simulationNodes) {
    if (!targetIdsSet.has(node.id)) continue
    result.set(node.id, {
      x: Number.isFinite(node.x) ? Math.round(node.x as number) : 0,
      y: Number.isFinite(node.y) ? Math.round(node.y as number) : 0,
      width: nodeById.get(node.id)?.width || BOARD_NODE_WIDTH,
      ...(node.pinned ? { pinned: true } : {}),
    })
  }
  return result
}
