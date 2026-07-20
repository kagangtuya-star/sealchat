import * as THREE from 'three'

import type { Dice3DSkin } from '@/types'

export type StandardDiceType = 'd2' | 'd4' | 'd6' | 'd8' | 'd10' | 'd12' | 'd20'
export type DiceResourceKey = StandardDiceType | 'd100ones' | 'd100tens'

interface DiceFace {
	indices: number[]
	center: THREE.Vector3
	normal: THREE.Vector3
	value: number | null
	localUp: THREE.Vector3
	uvPoints: Array<{ u: number; v: number }>
	materialIndex: number
	vertexValues?: number[]
}

interface DiceDefinition {
	type: StandardDiceType
	vertices: THREE.Vector3[]
	faces: DiceFace[]
	resultFaces: DiceFace[]
	radius: number
}

export interface DiceFaceDirection {
	value: number
	localNormal: THREE.Vector3
	localUp: THREE.Vector3
}

export interface DiceGeometryResource {
	geometry: THREE.BufferGeometry
	edgeGeometry: THREE.EdgesGeometry
	colliderVertices: Float32Array
	registryType: StandardDiceType
	atlasType: DiceAtlasType
	radius: number
}

export type DiceAtlasType = StandardDiceType | 'd100'

const UP_SCREEN = new THREE.Vector3(0, 0, -1)
const tempA = new THREE.Vector3()
const tempB = new THREE.Vector3()
const tempC = new THREE.Vector3()

const TYPE_META: Record<DiceAtlasType, { columns: number; rows: number; slots: number }> = {
	d2: { columns: 2, rows: 1, slots: 2 },
	d4: { columns: 2, rows: 2, slots: 4 },
	d6: { columns: 3, rows: 2, slots: 6 },
	d8: { columns: 4, rows: 2, slots: 8 },
	d10: { columns: 5, rows: 2, slots: 10 },
	d12: { columns: 4, rows: 3, slots: 12 },
	d20: { columns: 5, rows: 4, slots: 20 },
	d100: { columns: 5, rows: 4, slots: 20 },
}

const DEFAULT_LABELS: Record<DiceAtlasType, string[]> = {
	d2: ['1', '2'],
	d4: ['1', '2', '3', '4'],
	d6: ['1', '2', '3', '4', '5', '6'],
	d8: Array.from({ length: 8 }, (_, index) => String(index + 1)),
	d10: ['1', '2', '3', '4', '5', '6', '7', '8', '9', '0'],
	d12: Array.from({ length: 12 }, (_, index) => String(index + 1)),
	d20: Array.from({ length: 20 }, (_, index) => String(index + 1)),
	d100: ['1', '2', '3', '4', '5', '6', '7', '8', '9', '0', '10', '20', '30', '40', '50', '60', '70', '80', '90', '00'],
}

const FACE_REGISTRY = {} as Record<StandardDiceType, DiceFaceDirection[]>

const orientFace = (vertices: THREE.Vector3[], sourceIndices: number[]) => {
	const indices = [...sourceIndices]
	const center = new THREE.Vector3()
	indices.forEach(index => center.add(vertices[index]))
	center.multiplyScalar(1 / indices.length)

	const normal = new THREE.Vector3()
	for (let index = 1; index < indices.length - 1; index += 1) {
		normal.crossVectors(
			tempA.copy(vertices[indices[index]]).sub(vertices[indices[0]]),
			tempB.copy(vertices[indices[index + 1]]).sub(vertices[indices[0]]),
		)
		if (normal.lengthSq() > 1e-10) break
	}
	normal.normalize()
	if (normal.dot(center) < 0) normal.multiplyScalar(-1)

	const axis = Math.abs(normal.y) < 0.88 ? tempC.set(0, 1, 0) : tempC.set(0, 0, -1)
	const basisX = new THREE.Vector3().crossVectors(axis, normal).normalize()
	const basisY = new THREE.Vector3().crossVectors(normal, basisX).normalize()
	indices.sort((left, right) => {
		const leftVector = tempA.copy(vertices[left]).sub(center)
		const rightVector = tempB.copy(vertices[right]).sub(center)
		return Math.atan2(leftVector.dot(basisY), leftVector.dot(basisX))
			- Math.atan2(rightVector.dot(basisY), rightVector.dot(basisX))
	})
	if (indices.length >= 3) {
		tempA.copy(vertices[indices[1]]).sub(vertices[indices[0]])
		tempB.copy(vertices[indices[2]]).sub(vertices[indices[1]])
		if (tempA.cross(tempB).dot(normal) < 0) indices.reverse()
	}
	return { indices, center, normal }
}

const uniqueVertices = (geometry: THREE.BufferGeometry) => {
	const position = geometry.getAttribute('position')
	const map = new Map<string, THREE.Vector3>()
	for (let index = 0; index < position.count; index += 1) {
		const vertex = new THREE.Vector3(position.getX(index), position.getY(index), position.getZ(index))
		const key = `${vertex.x.toFixed(6)},${vertex.y.toFixed(6)},${vertex.z.toFixed(6)}`
		if (!map.has(key)) map.set(key, vertex)
	}
	geometry.dispose()
	return [...map.values()]
}

const extractConvexFaces = (vertices: THREE.Vector3[], epsilon = 1e-4) => {
	const faces = new Map<string, ReturnType<typeof orientFace>>()
	for (let i = 0; i < vertices.length - 2; i += 1) {
		for (let j = i + 1; j < vertices.length - 1; j += 1) {
			for (let k = j + 1; k < vertices.length; k += 1) {
				tempA.crossVectors(tempB.copy(vertices[j]).sub(vertices[i]), tempC.copy(vertices[k]).sub(vertices[i]))
				if (tempA.lengthSq() < 1e-10) continue
				tempA.normalize()
				let distance = tempA.dot(vertices[i])
				let positive = false
				let negative = false
				for (const vertex of vertices) {
					const side = tempA.dot(vertex) - distance
					if (side > epsilon) positive = true
					else if (side < -epsilon) negative = true
					if (positive && negative) break
				}
				if (positive && negative) continue
				if (positive) {
					tempA.multiplyScalar(-1)
					distance *= -1
				}
				const indices = vertices
					.map((vertex, index) => Math.abs(tempA.dot(vertex) - distance) <= epsilon * 4 ? index : -1)
					.filter(index => index >= 0)
				if (indices.length < 3) continue
				const key = [...indices].sort((left, right) => left - right).join(',')
				if (!faces.has(key)) faces.set(key, orientFace(vertices, indices))
			}
		}
	}
	return [...faces.values()]
}

const computeFaceUv = (type: StandardDiceType, vertices: THREE.Vector3[], face: ReturnType<typeof orientFace>) => {
	const localUp = new THREE.Vector3()
	if (type === 'd10') {
		let longest = face.indices[0]
		for (const index of face.indices) {
			if (vertices[index].distanceToSquared(face.center) > vertices[longest].distanceToSquared(face.center)) longest = index
		}
		localUp.copy(vertices[longest]).sub(face.center).normalize()
	} else {
		localUp.copy(UP_SCREEN).addScaledVector(face.normal, -UP_SCREEN.dot(face.normal))
	}
	if (localUp.lengthSq() < 0.04) localUp.set(0, 1, 0).addScaledVector(face.normal, -face.normal.y)
	if (localUp.lengthSq() < 0.04) localUp.set(1, 0, 0).addScaledVector(face.normal, -face.normal.x)
	localUp.normalize()
	const localRight = new THREE.Vector3().crossVectors(localUp, face.normal).normalize()
	let maxExtent = 0
	const points = face.indices.map(index => {
		const relative = new THREE.Vector3().copy(vertices[index]).sub(face.center)
		const point = { x: relative.dot(localRight), y: relative.dot(localUp) }
		maxExtent = Math.max(maxExtent, Math.abs(point.x), Math.abs(point.y))
		return point
	})
	const scale = maxExtent > 1e-8 ? 0.42 / maxExtent : 1
	return { localUp, points: points.map(point => ({ u: 0.5 + point.x * scale, v: 0.5 + point.y * scale })) }
}

const buildDefinition = (
	type: StandardDiceType,
	vertices: THREE.Vector3[],
	faceIndices: number[][],
	resultFaceCount = faceIndices.length,
): DiceDefinition => {
	const oriented = faceIndices.map(indices => orientFace(vertices, indices))
	const resultFaces = oriented.slice(0, resultFaceCount) as DiceFace[]
	const cosmeticFaces = oriented.slice(resultFaceCount) as DiceFace[]
	if (type === 'd6') {
		const wanted = [
			{ value: 1, normal: new THREE.Vector3(0, 1, 0) }, { value: 6, normal: new THREE.Vector3(0, -1, 0) },
			{ value: 2, normal: new THREE.Vector3(0, 0, 1) }, { value: 5, normal: new THREE.Vector3(0, 0, -1) },
			{ value: 3, normal: new THREE.Vector3(1, 0, 0) }, { value: 4, normal: new THREE.Vector3(-1, 0, 0) },
		]
		const remaining = new Set(resultFaces)
		const ordered: DiceFace[] = []
		for (const item of wanted) {
			let best: DiceFace | null = null
			let bestDot = -Infinity
			for (const face of remaining) {
				const dot = face.normal.dot(item.normal)
				if (dot > bestDot) { bestDot = dot; best = face }
			}
			if (!best) continue
			remaining.delete(best)
			best.value = item.value
			ordered.push(best)
		}
		resultFaces.splice(0, resultFaces.length, ...ordered.sort((left, right) => (left.value || 0) - (right.value || 0)))
	} else if (type === 'd2') {
		resultFaces.sort((left, right) => right.normal.y - left.normal.y)
		resultFaces.forEach((face, index) => { face.value = index + 1 })
	} else {
		resultFaces.sort((left, right) => {
			const height = right.center.y - left.center.y
			return Math.abs(height) > 1e-6 ? height : Math.atan2(left.center.z, left.center.x) - Math.atan2(right.center.z, right.center.x)
		})
		resultFaces.forEach((face, index) => { face.value = index + 1 })
	}
	for (const face of resultFaces) {
		const uv = computeFaceUv(type, vertices, face)
		face.localUp = uv.localUp
		face.uvPoints = uv.points
		face.materialIndex = 0
	}
	for (const face of cosmeticFaces) {
		face.value = null
		face.localUp = new THREE.Vector3(0, 1, 0)
		face.uvPoints = face.indices.map(() => ({ u: 0.5, v: 0.5 }))
		face.materialIndex = 1
	}
	FACE_REGISTRY[type] = resultFaces
		.slice().sort((left, right) => (left.value || 0) - (right.value || 0))
		.map(face => ({ value: face.value!, localNormal: face.normal.clone(), localUp: face.localUp.clone() }))
	return {
		type,
		vertices,
		faces: [...resultFaces, ...cosmeticFaces],
		resultFaces,
		radius: vertices.reduce((maximum, vertex) => Math.max(maximum, vertex.length()), 0),
	}
}

const createBeveledDefinition = (type: StandardDiceType, sourceVertices: THREE.Vector3[], faceIndices: number[][], bevel: number) => {
	const sourceFaces = faceIndices.map(indices => orientFace(sourceVertices, indices))
	const vertices: THREE.Vector3[] = []
	const mainFaces: number[][] = []
	const edgeUses = new Map<string, { insetA: number; insetB: number }>()
	const edgeFaces: number[][] = []
	const caps = Array.from({ length: sourceVertices.length }, () => [] as number[])
	for (const sourceFace of sourceFaces) {
		const inset = new Map<number, number>()
		const main = sourceFace.indices.map(sourceIndex => {
			const index = vertices.length
			vertices.push(sourceVertices[sourceIndex].clone().lerp(sourceFace.center, bevel))
			inset.set(sourceIndex, index)
			caps[sourceIndex].push(index)
			return index
		})
		mainFaces.push(main)
		for (let index = 0; index < sourceFace.indices.length; index += 1) {
			const a = sourceFace.indices[index]
			const b = sourceFace.indices[(index + 1) % sourceFace.indices.length]
			const key = a < b ? `${a},${b}` : `${b},${a}`
			const current = { insetA: inset.get(a)!, insetB: inset.get(b)! }
			const previous = edgeUses.get(key)
			if (previous) edgeFaces.push([previous.insetA, previous.insetB, current.insetA, current.insetB])
			else edgeUses.set(key, current)
		}
	}
	return buildDefinition(type, vertices, [...mainFaces, ...edgeFaces, ...caps.filter(indices => indices.length >= 3)], mainFaces.length)
}

const createD2 = () => {
	const segments = 32
	const vertices: THREE.Vector3[] = []
	for (const y of [0.105, -0.105]) {
		for (let index = 0; index < segments; index += 1) {
			const angle = index / segments * Math.PI * 2
			vertices.push(new THREE.Vector3(Math.cos(angle) * 0.62, y, Math.sin(angle) * 0.62))
		}
	}
	const faces = [
		Array.from({ length: segments }, (_, index) => index),
		Array.from({ length: segments }, (_, index) => segments + index).reverse(),
	]
	for (let index = 0; index < segments; index += 1) faces.push([index, (index + 1) % segments, segments + (index + 1) % segments, segments + index])
	return buildDefinition('d2', vertices, faces, 2)
}

const createD6 = () => {
	const h = 0.46
	const vertices = [
		new THREE.Vector3(-h, -h, -h), new THREE.Vector3(h, -h, -h), new THREE.Vector3(h, h, -h), new THREE.Vector3(-h, h, -h),
		new THREE.Vector3(-h, -h, h), new THREE.Vector3(h, -h, h), new THREE.Vector3(h, h, h), new THREE.Vector3(-h, h, h),
	]
	return buildDefinition('d6', vertices, [[3, 2, 6, 7], [4, 5, 1, 0], [4, 7, 6, 5], [1, 2, 3, 0], [5, 6, 2, 1], [0, 3, 7, 4]])
}

const createD10 = () => {
	const primal: THREE.Vector3[] = []
	for (let index = 0; index < 5; index += 1) {
		const angle = index / 5 * Math.PI * 2
		primal.push(new THREE.Vector3(Math.cos(angle), 0.85, Math.sin(angle)))
	}
	for (let index = 0; index < 5; index += 1) {
		const angle = (index + 0.5) / 5 * Math.PI * 2
		primal.push(new THREE.Vector3(Math.cos(angle), -0.85, Math.sin(angle)))
	}
	const primalFaces: number[][] = [[0, 1, 2, 3, 4], [9, 8, 7, 6, 5]]
	for (let index = 0; index < 5; index += 1) {
		primalFaces.push([index, (index + 1) % 5, 5 + index], [index, 5 + index, 5 + (index + 4) % 5])
	}
	const dualVertices = primalFaces.map(indices => {
		const face = orientFace(primal, indices)
		return face.normal.clone().multiplyScalar(1 / face.normal.dot(primal[indices[0]]))
	})
	const dualFaces = primal.map((_, vertexIndex) => {
		const incident = primalFaces.map((face, faceIndex) => face.includes(vertexIndex) ? faceIndex : -1).filter(index => index >= 0)
		return orientFace(dualVertices, incident).indices
	})
	const scale = 0.73 / dualVertices.reduce((maximum, vertex) => Math.max(maximum, vertex.length()), 0)
	dualVertices.forEach(vertex => vertex.multiplyScalar(scale))
	return createBeveledDefinition('d10', dualVertices, dualFaces, 0.105)
}

const platonic = (type: StandardDiceType, geometry: THREE.BufferGeometry, bevel = 0) => {
	const vertices = uniqueVertices(geometry)
	const faces = extractConvexFaces(vertices).map(face => face.indices)
	return bevel ? createBeveledDefinition(type, vertices, faces, bevel) : buildDefinition(type, vertices, faces)
}

const DEFINITIONS: Record<StandardDiceType, DiceDefinition> = {
	d2: createD2(),
	d4: platonic('d4', new THREE.TetrahedronGeometry(0.72, 0)),
	d6: createD6(),
	d8: platonic('d8', new THREE.OctahedronGeometry(0.70, 0), 0.14),
	d10: createD10(),
	d12: platonic('d12', new THREE.DodecahedronGeometry(0.67, 0)),
	d20: platonic('d20', new THREE.IcosahedronGeometry(0.66, 0)),
}

const configureD4 = () => {
	const definition = DEFINITIONS.d4
	const ordered = definition.vertices.map((vertex, index) => ({ vertex, index })).sort((left, right) => {
		const height = right.vertex.y - left.vertex.y
		return Math.abs(height) > 1e-6 ? height : Math.atan2(left.vertex.z, left.vertex.x) - Math.atan2(right.vertex.z, right.vertex.x)
	})
	const values = new Map<number, number>()
	ordered.forEach((entry, index) => values.set(entry.index, index + 1))
	definition.resultFaces.forEach(face => { face.vertexValues = face.indices.map(index => values.get(index)!) })
	FACE_REGISTRY.d4 = ordered.map((entry, index) => {
		const localNormal = entry.vertex.clone().normalize()
		const anchor = ordered.find(candidate => candidate.index !== entry.index)!.vertex
		const localUp = anchor.clone().sub(entry.vertex).addScaledVector(localNormal, -anchor.clone().sub(entry.vertex).dot(localNormal)).normalize().multiplyScalar(-1)
		return { value: index + 1, localNormal, localUp }
	})
}
configureD4()

const atlasUv = (type: DiceAtlasType, cellIndex: number, point: { u: number; v: number }) => {
	const meta = TYPE_META[type]
	const column = cellIndex % meta.columns
	const row = Math.floor(cellIndex / meta.columns)
	return { u: (column + point.u) / meta.columns, v: 1 - (row + 1 - point.v) / meta.rows }
}

const buildGeometry = (definition: DiceDefinition, atlasType: DiceAtlasType, offset: number) => {
	const positions: number[] = []
	const normals: number[] = []
	const uvs: number[] = []
	const indices: number[] = []
	const groups: Array<{ start: number; count: number; materialIndex: number }> = []
	for (const face of definition.faces) {
		const base = positions.length / 3
		positions.push(face.center.x, face.center.y, face.center.z)
		normals.push(face.normal.x, face.normal.y, face.normal.z)
		const centerUv = face.value == null ? { u: 0.5, v: 0.5 } : atlasUv(atlasType, offset + face.value - 1, { u: 0.5, v: 0.5 })
		uvs.push(centerUv.u, centerUv.v)
		face.indices.forEach((vertexIndex, index) => {
			const vertex = definition.vertices[vertexIndex]
			positions.push(vertex.x, vertex.y, vertex.z)
			normals.push(face.normal.x, face.normal.y, face.normal.z)
			const uv = face.value == null ? { u: 0.5, v: 0.5 } : atlasUv(atlasType, offset + face.value - 1, face.uvPoints[index])
			uvs.push(uv.u, uv.v)
		})
		const start = indices.length
		face.indices.forEach((_, index) => indices.push(base, base + 1 + index, base + 1 + (index + 1) % face.indices.length))
		groups.push({ start, count: face.indices.length * 3, materialIndex: face.materialIndex })
	}
	const geometry = new THREE.BufferGeometry()
	geometry.setAttribute('position', new THREE.Float32BufferAttribute(positions, 3))
	geometry.setAttribute('normal', new THREE.Float32BufferAttribute(normals, 3))
	geometry.setAttribute('uv', new THREE.Float32BufferAttribute(uvs, 2))
	geometry.setIndex(indices)
	groups.forEach(group => geometry.addGroup(group.start, group.count, group.materialIndex))
	geometry.computeBoundingSphere()
	return geometry
}

const resource = (type: StandardDiceType, atlasType: DiceAtlasType = type, offset = 0): DiceGeometryResource => {
	const definition = DEFINITIONS[type]
	const geometry = buildGeometry(definition, atlasType, offset)
	return {
		geometry,
		edgeGeometry: new THREE.EdgesGeometry(geometry, type === 'd2' ? 18 : 7),
		colliderVertices: new Float32Array(definition.vertices.flatMap(vertex => [vertex.x, vertex.y, vertex.z])),
		registryType: type,
		atlasType,
		radius: definition.radius,
	}
}

export const DICE_GEOMETRY_RESOURCES: Record<DiceResourceKey, DiceGeometryResource> = {
	d2: resource('d2'), d4: resource('d4'), d6: resource('d6'), d8: resource('d8'),
	d10: resource('d10'), d12: resource('d12'), d20: resource('d20'),
	d100ones: resource('d10', 'd100', 0), d100tens: resource('d10', 'd100', 10),
}

export const getFaceDirection = (type: StandardDiceType, value: number) => FACE_REGISTRY[type].find(face => face.value === value)

export const detectTopFace = (type: StandardDiceType, quaternion: THREE.Quaternion) => {
	const faces = FACE_REGISTRY[type]
	let bestValue = faces[0].value
	let bestHeight = -Infinity
	for (const face of faces) {
		const height = tempA.copy(face.localNormal).applyQuaternion(quaternion).y
		if (height > bestHeight) {
			bestHeight = height
			bestValue = face.value
		}
	}
	return bestValue
}

const polygonPath = (context: CanvasRenderingContext2D, points: Array<{ u: number; v: number }>, x: number, y: number, cell: number) => {
	context.beginPath()
	points.forEach((point, index) => {
		const px = x + point.u * cell
		const py = y + (1 - point.v) * cell
		if (index === 0) context.moveTo(px, py)
		else context.lineTo(px, py)
	})
	context.closePath()
}

export const createDiceAtlasCanvas = (type: DiceAtlasType, skin: Dice3DSkin) => {
	const meta = TYPE_META[type]
	const cell = 192
	const canvas = document.createElement('canvas')
	canvas.width = meta.columns * cell
	canvas.height = meta.rows * cell
	const context = canvas.getContext('2d')!
	context.textAlign = 'center'
	context.textBaseline = 'middle'
	context.lineJoin = 'round'
	const polygons = type === 'd100'
		? [...DEFINITIONS.d10.resultFaces, ...DEFINITIONS.d10.resultFaces]
		: DEFINITIONS[type].resultFaces
	for (let index = 0; index < meta.slots; index += 1) {
		const x = index % meta.columns * cell
		const y = Math.floor(index / meta.columns) * cell
		const face = polygons[index]
		polygonPath(context, face.uvPoints, x, y, cell)
		context.fillStyle = skin.faceBackground || '#f5f6fa'
		context.fill()
		context.strokeStyle = skin.edgeColor || '#d1d5db'
		context.lineWidth = 7
		context.stroke()
		context.fillStyle = skin.faceForeground || '#111827'
		context.strokeStyle = 'rgba(0,0,0,.24)'
		context.lineWidth = 5
		if (type === 'd4') {
			face.vertexValues?.forEach((value, vertexIndex) => {
				const point = face.uvPoints[vertexIndex]
				const px = x + (0.5 + (point.u - 0.5) * 0.47) * cell
				const py = y + (1 - (0.5 + (point.v - 0.5) * 0.47)) * cell
				context.save()
				context.translate(px, py)
				context.rotate(Math.atan2(py - (y + cell / 2), px - (x + cell / 2)) + Math.PI / 2)
				context.font = '800 42px system-ui, sans-serif'
				context.strokeText(String(value), 0, 0)
				context.fillText(String(value), 0, 0)
				context.restore()
			})
			continue
		}
		const label = DEFAULT_LABELS[type][index]
		const kite = type === 'd10' || type === 'd100'
		const size = kite ? (label.length > 1 ? 54 : 70) : label.length > 1 ? 64 : 84
		context.font = `800 ${size}px system-ui, sans-serif`
		context.strokeText(label, x + cell / 2, y + cell / 2 + (kite ? 4 : 2), cell * (kite ? 0.48 : 0.64))
		context.fillText(label, x + cell / 2, y + cell / 2 + (kite ? 4 : 2), cell * (kite ? 0.48 : 0.64))
	}
	return canvas
}

export const createDiceAtlasTexture = (type: DiceAtlasType, skin: Dice3DSkin) => {
	const texture = new THREE.CanvasTexture(createDiceAtlasCanvas(type, skin))
	texture.colorSpace = THREE.SRGBColorSpace
	texture.wrapS = THREE.ClampToEdgeWrapping
	texture.wrapT = THREE.ClampToEdgeWrapping
	texture.minFilter = THREE.LinearMipmapLinearFilter
	texture.magFilter = THREE.LinearFilter
	return texture
}
