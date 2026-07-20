import * as THREE from 'three'
import RAPIER from '@dimforge/rapier3d-compat'

import type { Dice3DSkin, DiceVisualPayload } from '@/types'
import {
	createDiceAtlasTexture,
	DICE_GEOMETRY_RESOURCES,
	detectTopFace,
	getFaceDirection,
	type DiceGeometryResource,
	type DiceResourceKey,
} from './DiceGeometryRegistry'

interface ActiveDie {
	id: string
	mesh: THREE.Mesh
	body: RAPIER.RigidBody
	collider: RAPIER.Collider
	expiresAt: number
	interactive: boolean
	stableTime: number
	stableFace: number | null
	settled: boolean
	targetValue: number
	authoritative: boolean
	visualOffset: THREE.Quaternion
	previousPosition: THREE.Vector3
	previousQuaternion: THREE.Quaternion
	resource: DiceGeometryResource
	scale: number
}

interface DragState {
	die: ActiveDie
	target: THREE.Vector3
	smoothedTarget: THREE.Vector3
	anchorBody: RAPIER.RigidBody
	joint: RAPIER.ImpulseJoint
	localAnchor: THREE.Vector3
	samples: Array<{ position: THREE.Vector3, time: number }>
	lastTime: number
	pointerId: number
	startClient: THREE.Vector2
	moved: boolean
	shield: HTMLDivElement
}

interface ThrowPlan {
	edge: 'top' | 'right' | 'bottom' | 'left'
	inward: THREE.Vector3
	tangent: THREE.Vector3
}

const FIXED_STEP = 1 / 120
const MAX_PREDICTION_STEPS = 1200
const MAX_FRAME_STEPS = 8
const GRAVITY = 18
const AIR_LINEAR_DAMPING = 0.06
const AIR_ANGULAR_DAMPING = 0.14
const HELD_LINEAR_DAMPING = 0.35
const HELD_ANGULAR_DAMPING = 1.8
const UP = new THREE.Vector3(0, 1, 0)
const SCREEN_UP = new THREE.Vector3(0, 0, -1)
const tempQuaternion = new THREE.Quaternion()
const tempQuaternionB = new THREE.Quaternion()
const tempQuaternionC = new THREE.Quaternion()
const tempVector = new THREE.Vector3()
const tempVectorB = new THREE.Vector3()

const FACE_COUNTS: Record<DiceGeometryResource['registryType'], number> = {
	d2: 2,
	d4: 4,
	d6: 6,
	d8: 8,
	d10: 10,
	d12: 12,
	d20: 20,
}

const clamp = (value: number, min: number, max: number) => Math.max(min, Math.min(max, value))

export class DiceArena {
  private readonly scene = new THREE.Scene()
  private readonly camera = new THREE.PerspectiveCamera(34, 1, 0.1, 100)
	private readonly renderer: THREE.WebGLRenderer
	private readonly raycaster = new THREE.Raycaster()
	private readonly pointer = new THREE.Vector2()
	private readonly dragPlane = new THREE.Plane(new THREE.Vector3(0, 1, 0), 0)
	private world: RAPIER.World | null = null
  private dice: ActiveDie[] = []
  private boundaries: RAPIER.Collider[] = []
		private textures = new Map<string, THREE.Texture>()
  private frame = 0
  private lastFrame = performance.now()
  private accumulator = 0
	private width = 1
		private height = 1
		private wallBounce = 0.48
		private drag: DragState | null = null
		private pendingThrows: DiceVisualPayload[] = []
		private activeThrowDice: ActiveDie[] = []
		private boundaryDirty = false
		private arenaHalfWidth = 4.4
		private arenaHalfDepth = 4.4
		private symmetryRotations = new Map<DiceGeometryResource['registryType'], THREE.Quaternion[]>()
		private disposed = false

  constructor(private readonly canvas: HTMLCanvasElement) {
    this.renderer = new THREE.WebGLRenderer({ canvas, alpha: true, antialias: true })
    this.renderer.setClearColor(0x000000, 0)
    this.renderer.shadowMap.enabled = true
    this.renderer.shadowMap.type = THREE.PCFSoftShadowMap
			// 垂直俯视保持桌面坐标与屏幕拖拽方向一致。
		this.camera.up.set(0, 0, -1)
		this.camera.position.set(0, 14, 0)
		this.camera.lookAt(0, 0, 0)
    this.scene.add(new THREE.HemisphereLight(0xffffff, 0x293241, 2.4))
    const light = new THREE.DirectionalLight(0xffffff, 3.2)
    light.position.set(-4, 10, 5)
		light.castShadow = true
		this.scene.add(light)
		window.addEventListener('pointerdown', this.handlePointerDown, true)
  }

  async init() {
	    await RAPIER.init()
	    if (this.disposed) return
	    this.world = new RAPIER.World({ x: 0, y: -GRAVITY, z: 0 })
			this.world.timestep = FIXED_STEP
			this.world.numSolverIterations = 8
			this.world.numInternalPgsIterations = 2
			this.world.maxCcdSubsteps = 2
	    this.rebuildBoundaries()
	    this.frame = requestAnimationFrame(this.tick)
  }

  resize(width: number, height: number) {
    this.width = Math.max(1, width)
    this.height = Math.max(1, height)
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2))
		this.renderer.setSize(this.width, this.height, false)
		this.camera.aspect = this.width / this.height
		const portraitScale = this.camera.aspect < 1 ? Math.min(1 / this.camera.aspect, 1.8) : 1
		this.camera.position.set(0, 14 * portraitScale, 0)
		this.camera.lookAt(0, 0, 0)
		this.camera.updateProjectionMatrix()
			if (this.activeThrowDice.length > 0) this.boundaryDirty = true
			else this.rebuildBoundaries()
	  }

		play(payload: DiceVisualPayload) {
			if (!this.world || this.disposed) return
			this.pendingThrows.push(payload)
			this.tryStartNextThrow()
		}

		private startThrow(payload: DiceVisualPayload) {
			if (!this.world) return
			this.playThrowSound(payload)
		const nextWallBounce = Math.max(0, Math.min(0.95, payload.motion.wallBounce ?? 0.48))
		if (nextWallBounce !== this.wallBounce) {
			this.wallBounce = nextWallBounce
				this.rebuildBoundaries()
			}
    const maxDice = Math.max(1, Math.min(100, payload.motion.maxDice || 60))
    const requested = payload.groups.reduce((sum, group) => (
      sum + group.results.length * (group.type === 'd100' ? 2 : 1)
    ), 0)
    const available = Math.max(0, maxDice - this.dice.length)
    if (available < requested) this.removeOldestSettled(requested - available)

	    let index = 0
			const spawned: ActiveDie[] = []
		    const total = Math.min(requested, Math.max(0, maxDice - this.dice.length))
		    const random = seededRandom(payload.seed)
				const throwPlan = this.buildThrowPlan(random, payload.motion.entryEdge)
				for (const group of payload.groups) {
					for (const result of group.results) {
	        if (index >= total) break
						if (group.type === 'd100') {
							const normalized = result === 100 ? 0 : result
							const tens = Math.floor(normalized / 10)
							const ones = normalized % 10
								spawned.push(this.spawn('d100tens', tens === 0 ? 10 : tens, payload, index++, total, random, throwPlan))
								if (index < total) spawned.push(this.spawn('d100ones', ones === 0 ? 10 : ones, payload, index++, total, random, throwPlan))
						} else {
							if (group.type in DICE_GEOMETRY_RESOURCES) {
									spawned.push(this.spawn(group.type as DiceResourceKey, result, payload, index++, total, random, throwPlan))
							}
	        }
	      }
	    }
			if (spawned.length === 0) {
				this.tryStartNextThrow()
				return
			}
				try {
					this.predictOutcomeOffsets(spawned)
				} catch (error) {
					spawned.forEach(die => this.removeDie(die))
					this.dice = this.dice.filter(die => !spawned.includes(die))
					console.error('3D 骰子出目预计算失败，已取消本次视觉投掷', error)
					this.tryStartNextThrow()
					return
				}
				this.activeThrowDice = spawned
	  }

  dispose() {
    this.disposed = true
    cancelAnimationFrame(this.frame)
    this.clear()
			this.textures.forEach(texture => texture.dispose())
		this.textures.clear()
		this.renderer.dispose()
		window.removeEventListener('pointerdown', this.handlePointerDown, true)
		this.endDrag(false)
		this.world = null
  }

		clear() {
			if (!this.world) return
			this.endDrag(false)
			this.pendingThrows = []
			this.activeThrowDice = []
			this.dice.forEach(die => this.removeDie(die))
	    this.dice = []
	  }

		private buildThrowPlan(random: () => number, requestedEdge?: DiceVisualPayload['motion']['entryEdge']): ThrowPlan {
			const edgeIndex = requestedEdge && requestedEdge !== 'random'
				? ['top', 'right', 'bottom', 'left'].indexOf(requestedEdge)
				: Math.floor(random() * 4)
			if (edgeIndex === 0) return {
				edge: 'top',
				inward: new THREE.Vector3(0, 0, 1),
				tangent: new THREE.Vector3(1, 0, 0),
			}
			if (edgeIndex === 1) return {
				edge: 'right',
				inward: new THREE.Vector3(-1, 0, 0),
				tangent: new THREE.Vector3(0, 0, 1),
			}
			if (edgeIndex === 2) return {
				edge: 'bottom',
				inward: new THREE.Vector3(0, 0, -1),
				tangent: new THREE.Vector3(-1, 0, 0),
			}
			return {
				edge: 'left',
				inward: new THREE.Vector3(1, 0, 0),
				tangent: new THREE.Vector3(0, 0, -1),
			}
		}

			private spawn(
			resourceKey: DiceResourceKey,
			targetValue: number,
			payload: DiceVisualPayload,
			index: number,
			total: number,
			random: () => number,
				throwPlan: ThrowPlan,
		): ActiveDie {
			if (!this.world) throw new Error('3D 骰子物理世界尚未初始化')
			const resource = DICE_GEOMETRY_RESOURCES[resourceKey]
		const material = this.materialFor(resource, payload.appearance)
		const mesh = new THREE.Mesh(resource.geometry, material)
		const edgeMaterial = new THREE.LineBasicMaterial({
			color: payload.appearance.edgeColor || '#20242b',
			transparent: true,
			opacity: 0.72,
		})
		const edges = new THREE.LineSegments(resource.edgeGeometry, edgeMaterial)
		edges.raycast = () => undefined
		mesh.add(edges)
		mesh.castShadow = true
		mesh.receiveShadow = true
		mesh.frustumCulled = false
		const scale = clamp(payload.appearance.scale || 1, 0.5, 2)
		mesh.scale.setScalar(scale)

				const radius = resource.radius * scale
				const spacing = Math.max(0.9, radius * 2.25)
				const tangentHalfExtent = throwPlan.edge === 'left' || throwPlan.edge === 'right'
					? this.arenaHalfDepth
					: this.arenaHalfWidth
				const inwardHalfExtent = throwPlan.edge === 'left' || throwPlan.edge === 'right'
					? this.arenaHalfWidth
					: this.arenaHalfDepth
				const slotsPerRow = Math.max(1, Math.floor((tangentHalfExtent * 2 - radius * 2) / spacing) + 1)
				const inwardRows = Math.max(1, Math.floor(Math.min(inwardHalfExtent * 1.4, inwardHalfExtent * 2 - radius * 2 - 0.5) / spacing) + 1)
				const layerCapacity = Math.max(1, slotsPerRow * inwardRows)
				const layer = Math.floor(index / layerCapacity)
				const layerIndex = index % layerCapacity
				const inwardRow = Math.floor(layerIndex / slotsPerRow)
				const slot = layerIndex % slotsPerRow
					const usedSlots = Math.max(1, Math.min(slotsPerRow, total - layer * layerCapacity - inwardRow * slotsPerRow))
				const tangentCoordinate = clamp(
					(slot - (usedSlots - 1) / 2) * spacing + (random() - 0.5) * spacing * 0.06,
					-tangentHalfExtent + radius + 0.22,
					tangentHalfExtent - radius - 0.22,
				)
				const edgeInset = radius + 0.24
				let x = throwPlan.edge === 'left'
					? -this.arenaHalfWidth + edgeInset
					: throwPlan.edge === 'right'
						? this.arenaHalfWidth - edgeInset
						: tangentCoordinate
				let z = throwPlan.edge === 'top'
					? -this.arenaHalfDepth + edgeInset
					: throwPlan.edge === 'bottom'
						? this.arenaHalfDepth - edgeInset
						: tangentCoordinate
				x += throwPlan.inward.x * inwardRow * spacing
				z += throwPlan.inward.z * inwardRow * spacing
				x = clamp(x, -this.arenaHalfWidth + edgeInset, this.arenaHalfWidth - edgeInset)
				z = clamp(z, -this.arenaHalfDepth + edgeInset, this.arenaHalfDepth - edgeInset)
				const y = 2.5 + layer * spacing * 1.12 + random() * 0.25
	    mesh.position.set(x, y, z)
    mesh.rotation.set(random() * Math.PI, random() * Math.PI, random() * Math.PI)
    this.scene.add(mesh)

		const initialQuaternion = new THREE.Quaternion().setFromEuler(mesh.rotation)
		const body = this.world.createRigidBody(
				RAPIER.RigidBodyDesc.dynamic()
					.setTranslation(x, y, z)
					.setRotation(initialQuaternion)
		        .setLinearDamping(AIR_LINEAR_DAMPING)
		        .setAngularDamping(AIR_ANGULAR_DAMPING)
					.setAdditionalSolverIterations(2)
	        .setCcdEnabled(true),
	    )
		const colliderVertices = new Float32Array(resource.colliderVertices.length)
		for (let vertexIndex = 0; vertexIndex < resource.colliderVertices.length; vertexIndex += 1) {
			colliderVertices[vertexIndex] = resource.colliderVertices[vertexIndex] * scale
		}
				const collider = RAPIER.ColliderDesc.convexHull(colliderVertices)
			if (!collider) throw new Error(`无法创建骰子碰撞体：${resourceKey}`)
			collider
				.setDensity(1)
				.setFriction(0.32)
				.setRestitution(0.30)
				.setFrictionCombineRule(RAPIER.CoefficientCombineRule.Average)
				.setRestitutionCombineRule(RAPIER.CoefficientCombineRule.Average)
			const createdCollider = this.world.createCollider(collider, body)
	    const speed = Math.max(0.25, Math.min(3, payload.motion.speed || 1))
	    const force = Math.max(0.25, Math.min(3, payload.motion.throwForce || 1))
				const direction = throwPlan.inward.clone()
					.addScaledVector(throwPlan.tangent, (random() - 0.5) * 0.42)
					.normalize()
				const horizontalSpeed = (2.3 + force * 1.7) * speed
		    body.setLinvel({
					x: direction.x * horizontalSpeed,
					y: 0.7 + force * 0.55 + (random() - 0.5) * 0.45,
					z: direction.z * horizontalSpeed,
			}, true)
			const angularSpeed = (7 + random() * 7) * speed
	    body.setAngvel({
				x: (random() * 2 - 1) * angularSpeed,
				y: (random() * 2 - 1) * angularSpeed,
				z: (random() * 2 - 1) * angularSpeed,
			}, true)

			const activeDie: ActiveDie = {
	      id: `${payload.rollId}:${index}`,
					mesh,
					body,
					collider: createdCollider,
				expiresAt: performance.now() + Math.max(1500, payload.motion.lingerMs || 8000) + 2600,
				interactive: payload.motion.interactive !== false,
				stableTime: 0,
				stableFace: null,
				settled: false,
				targetValue,
					authoritative: true,
					visualOffset: new THREE.Quaternion(),
					previousPosition: new THREE.Vector3(x, y, z),
					previousQuaternion: initialQuaternion.clone(),
				resource,
				scale,
			}
			this.dice.push(activeDie)
			return activeDie
		}

		private handlePointerDown = (event: PointerEvent) => {
				if (event.button !== 0 || this.drag || this.disposed || !this.world || this.dice.some(die => !die.settled)) return
		const rect = this.canvas.getBoundingClientRect()
		if (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom) return
		this.pointer.set(
			((event.clientX - rect.left) / Math.max(1, rect.width)) * 2 - 1,
			-((event.clientY - rect.top) / Math.max(1, rect.height)) * 2 + 1,
		)
		this.raycaster.setFromCamera(this.pointer, this.camera)
		const pickable = this.dice.filter(die => die.interactive && die.settled).map(die => die.mesh)
		const hit = this.raycaster.intersectObjects(pickable, false)[0]
		if (!hit) return
			const die = this.dice.find(item => item.mesh === hit.object)
			if (!die) return
			event.preventDefault()
			event.stopPropagation()
			const translation = die.body.translation()
			const rotation = die.body.rotation()
			const bodyPosition = new THREE.Vector3(translation.x, translation.y, translation.z)
			const bodyQuaternion = new THREE.Quaternion(rotation.x, rotation.y, rotation.z, rotation.w)
			const localAnchor = hit.point.clone().sub(bodyPosition).applyQuaternion(bodyQuaternion.clone().invert())
			const radius = die.resource.radius * die.scale
			const liftHeight = Math.max(hit.point.y + radius * 0.7, radius * 1.45)
			this.dragPlane.constant = -liftHeight
			const pointerPoint = new THREE.Vector3()
			if (!this.raycaster.ray.intersectPlane(this.dragPlane, pointerPoint)) return
			const target = this.clampDragTarget(pointerPoint, radius)
			const anchorBody = this.world.createRigidBody(
				RAPIER.RigidBodyDesc.kinematicPositionBased().setTranslation(hit.point.x, hit.point.y, hit.point.z),
			)
			const joint = this.world.createImpulseJoint(
				RAPIER.JointData.spherical(localAnchor, { x: 0, y: 0, z: 0 }),
				die.body,
				anchorBody,
				true,
				)
				die.body.setAdditionalSolverIterations(4)
				die.body.setLinearDamping(HELD_LINEAR_DAMPING)
				die.body.setAngularDamping(HELD_ANGULAR_DAMPING)
		const shield = document.createElement('div')
		shield.className = 'dice3d-drag-shield'
		Object.assign(shield.style, {
			position: 'fixed', inset: '0', zIndex: '9601', background: 'transparent',
			pointerEvents: 'auto', touchAction: 'none', cursor: 'grabbing',
		})
		document.body.appendChild(shield)
		window.addEventListener('pointermove', this.handleDragMove, true)
		window.addEventListener('pointerup', this.handleDragEnd, true)
		window.addEventListener('pointercancel', this.handleDragCancel, true)
			this.drag = {
				die,
				target,
				smoothedTarget: hit.point.clone(),
				anchorBody,
				joint,
				localAnchor,
				samples: [],
				lastTime: performance.now(),
				pointerId: event.pointerId,
					startClient: new THREE.Vector2(event.clientX, event.clientY),
				moved: false,
				shield,
		}
		die.settled = false
		die.stableTime = 0
			// 用户抓取发生在权威出目已经结算之后；此后只表现自由物理。
			die.authoritative = false
			die.expiresAt = Math.max(die.expiresAt, performance.now() + 5000)
			die.body.wakeUp()
		}

	private handleDragMove = (event: PointerEvent) => {
		if (!this.drag || event.pointerId !== this.drag.pointerId) return
		event.preventDefault()
		const totalMovement = Math.hypot(
			event.clientX - this.drag.startClient.x,
			event.clientY - this.drag.startClient.y,
		)
				if (!this.drag.moved && totalMovement < 5) {
				this.drag.lastTime = performance.now()
				return
		}
		this.drag.moved = true
		const rect = this.canvas.getBoundingClientRect()
		this.pointer.set(
			((event.clientX - rect.left) / Math.max(1, rect.width)) * 2 - 1,
			-((event.clientY - rect.top) / Math.max(1, rect.height)) * 2 + 1,
		)
		this.raycaster.setFromCamera(this.pointer, this.camera)
			const next = new THREE.Vector3()
			if (!this.raycaster.ray.intersectPlane(this.dragPlane, next)) return
			const radius = this.drag.die.resource.radius * this.drag.die.scale
			this.clampDragTarget(next, radius)
			const now = performance.now()
			this.drag.target.copy(next)
			const lastSample = this.drag.samples[this.drag.samples.length - 1]
			if (!lastSample || now - lastSample.time >= 32) {
				this.drag.samples.push({ position: next.clone(), time: now })
				while (this.drag.samples.length > 6) this.drag.samples.shift()
			}
			this.drag.lastTime = now
	}

	private handleDragEnd = (event: PointerEvent) => {
		if (this.drag && event.pointerId === this.drag.pointerId) this.endDrag(true)
	}

	private handleDragCancel = (event: PointerEvent) => {
		if (this.drag && event.pointerId === this.drag.pointerId) this.endDrag(false)
	}

		private endDrag(throwDie: boolean) {
			const drag = this.drag
				if (!drag) return
			window.removeEventListener('pointermove', this.handleDragMove, true)
			window.removeEventListener('pointerup', this.handleDragEnd, true)
				window.removeEventListener('pointercancel', this.handleDragCancel, true)
				drag.shield.remove()
			if (this.world) {
				this.world.removeImpulseJoint(drag.joint, true)
				this.world.removeRigidBody(drag.anchorBody)
			}
				drag.die.body.setAdditionalSolverIterations(2)
				drag.die.body.setLinearDamping(AIR_LINEAR_DAMPING)
				drag.die.body.setAngularDamping(AIR_ANGULAR_DAMPING)
			if (throwDie && drag.moved) {
				const velocity = this.computeDragVelocity(drag)
				const speed = Math.hypot(velocity.x, velocity.z)
				const rotation = drag.die.body.rotation()
				const position = drag.die.body.translation()
				const grabPoint = drag.localAnchor.clone()
					.applyQuaternion(new THREE.Quaternion(rotation.x, rotation.y, rotation.z, rotation.w))
					.add(new THREE.Vector3(position.x, position.y, position.z))
				const pointVelocity = drag.die.body.velocityAtPoint(grabPoint)
				const desiredVelocity = new THREE.Vector3(
					velocity.x,
					Math.max(pointVelocity.y, clamp(speed * 0.22, 0.3, 2.4)),
					velocity.z,
				)
				const impulse = desiredVelocity
					.sub(new THREE.Vector3(pointVelocity.x, pointVelocity.y, pointVelocity.z))
					.multiplyScalar(drag.die.body.mass())
					.clampLength(0, drag.die.body.mass() * 14)
				drag.die.body.applyImpulseAtPoint(impulse, grabPoint, true)
			}
			drag.die.expiresAt = Math.max(drag.die.expiresAt, performance.now() + 4000)
			this.drag = null
			this.tryStartNextThrow()
		}

		private clampDragTarget(target: THREE.Vector3, radius: number) {
			target.x = clamp(target.x, -this.arenaHalfWidth + radius, this.arenaHalfWidth - radius)
			target.z = clamp(target.z, -this.arenaHalfDepth + radius, this.arenaHalfDepth - radius)
			return target
		}

		private computeDragVelocity(drag: DragState) {
			const idleSeconds = Math.max(0, (performance.now() - drag.lastTime) / 1000)
			const samples = drag.samples.slice(-4)
			if (samples.length < 2) return new THREE.Vector3()
			const first = samples[0]
			const last = samples[samples.length - 1]
			const seconds = Math.max(1 / 240, (last.time - first.time) / 1000)
			return last.position.clone()
				.sub(first.position)
				.divideScalar(seconds)
				.multiplyScalar(clamp(1 - idleSeconds / 0.14, 0, 1))
				.clampLength(0, 14)
		}

			private updateDragConstraint() {
				if (!this.drag) return
				const delta = this.drag.target.clone().sub(this.drag.smoothedTarget)
				const moving = delta.lengthSq() > 1e-8
				const maxDistance = 18 * FIXED_STEP
				if (delta.lengthSq() > maxDistance * maxDistance) delta.setLength(maxDistance)
				this.drag.smoothedTarget.add(delta)
				this.drag.anchorBody.setNextKinematicTranslation(this.drag.smoothedTarget)
				if (moving) this.drag.die.body.wakeUp()
			}

	private materialFor(resource: DiceGeometryResource, skin: Dice3DSkin) {
		const textureSource = skin.textures?.[resource.atlasType]
		let map: THREE.Texture | null = null
		if (textureSource) {
			map = this.textures.get(textureSource) || null
			if (!map) {
				map = new THREE.TextureLoader().load(resolveDiceAssetURL(textureSource))
				map.colorSpace = THREE.SRGBColorSpace
				this.textures.set(textureSource, map)
			}
		} else {
			const textureKey = [resource.atlasType, skin.faceBackground, skin.faceForeground, skin.edgeColor].join(':')
			map = this.textures.get(textureKey) || null
			if (!map) {
				map = createDiceAtlasTexture(resource.atlasType, skin)
				map.anisotropy = Math.min(4, this.renderer.capabilities.getMaxAnisotropy())
				this.textures.set(textureKey, map)
			}
		}
		const faceMaterial = new THREE.MeshStandardMaterial({
			color: 0xffffff,
			map,
			roughness: skin.roughness ?? 0.72,
			metalness: skin.metalness ?? 0.05,
			flatShading: true,
		})
		const hasCosmeticFaces = resource.geometry.groups.some(group => group.materialIndex === 1)
		return hasCosmeticFaces ? [faceMaterial, new THREE.MeshStandardMaterial({
			color: new THREE.Color(skin.edgeColor || '#353b46'),
			roughness: 0.66,
			metalness: 0.08,
			flatShading: true,
		})] : faceMaterial
	}

	private playThrowSound(payload: DiceVisualPayload) {
		if (!payload.audio?.enabled || payload.audio.volume <= 0) return
		const volume = Math.max(0, Math.min(1, payload.audio.volume))
		if (payload.audio.soundAssetId) {
			const audio = new Audio(resolveDiceAssetURL(payload.audio.soundAssetId))
			audio.volume = volume
			void audio.play().catch(() => undefined)
			return
		}
		try {
			const AudioContextClass = window.AudioContext || (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
			if (!AudioContextClass) return
			const context = new AudioContextClass()
			const oscillator = context.createOscillator()
			const gain = context.createGain()
			oscillator.type = 'triangle'
			oscillator.frequency.setValueAtTime(150, context.currentTime)
			oscillator.frequency.exponentialRampToValueAtTime(55, context.currentTime + 0.11)
			gain.gain.setValueAtTime(volume * 0.12, context.currentTime)
			gain.gain.exponentialRampToValueAtTime(0.0001, context.currentTime + 0.12)
			oscillator.connect(gain).connect(context.destination)
			oscillator.start()
			oscillator.stop(context.currentTime + 0.12)
			oscillator.addEventListener('ended', () => void context.close(), { once: true })
		} catch {
			// 浏览器禁止自动音频时静默降级，不影响骰点消息。
		}
	}

		private buildFaceCanonicalQuaternion(type: DiceGeometryResource['registryType'], value: number) {
			const face = getFaceDirection(type, value)
			if (!face) throw new RangeError(`非法骰面点数：${type} ${value}`)
			tempQuaternionB.setFromUnitVectors(face.localNormal, UP)
		tempVector.copy(face.localUp).applyQuaternion(tempQuaternionB)
		tempVector.y = 0
		if (tempVector.lengthSq() < 1e-8) tempVector.copy(SCREEN_UP)
		else tempVector.normalize()
		tempVectorB.crossVectors(tempVector, SCREEN_UP)
		const yaw = Math.atan2(UP.dot(tempVectorB), clamp(tempVector.dot(SCREEN_UP), -1, 1))
		tempQuaternionC.setFromAxisAngle(UP, yaw)
			return tempQuaternionC.clone().multiply(tempQuaternionB).normalize()
		}

			private buildFaceSwapQuaternion(type: DiceGeometryResource['registryType'], rawValue: number, targetValue: number) {
				if (type === 'd2') {
					const rawCanonical = this.buildFaceCanonicalQuaternion(type, rawValue)
					const targetCanonical = this.buildFaceCanonicalQuaternion(type, targetValue)
					return rawCanonical.invert().multiply(targetCanonical).normalize()
				}
				const rawFace = getFaceDirection(type, rawValue)
				const targetFace = getFaceDirection(type, targetValue)
				if (!rawFace || !targetFace) throw new RangeError(`非法骰面映射：${type} ${rawValue} -> ${targetValue}`)
				const rotations = this.getSymmetryRotations(type)
				const candidates = rotations.filter(rotation => (
					targetFace.localNormal.clone().applyQuaternion(rotation).distanceToSquared(rawFace.localNormal) < 1e-8
				))
				candidates.sort((left, right) => (
					targetFace.localUp.clone().applyQuaternion(right).dot(rawFace.localUp)
					- targetFace.localUp.clone().applyQuaternion(left).dot(rawFace.localUp)
				))
				if (!candidates[0]) throw new Error(`找不到骰面几何对称旋转：${type} ${rawValue} -> ${targetValue}`)
				return candidates[0].clone()
			}

			private getSymmetryRotations(type: DiceGeometryResource['registryType']) {
				const cached = this.symmetryRotations.get(type)
				if (cached) return cached
				const resource = DICE_GEOMETRY_RESOURCES[type]
				const faces = Array.from({ length: FACE_COUNTS[type] }, (_, index) => getFaceDirection(type, index + 1))
					.filter((face): face is NonNullable<typeof face> => Boolean(face))
				const sourceA = faces[0]?.localNormal
				const sourceB = sourceA && faces.find(face => Math.abs(sourceA.dot(face.localNormal)) < 0.999)?.localNormal
				if (!sourceA || !sourceB) throw new Error(`无法构建骰子对称群：${type}`)
				const sourceDot = sourceA.dot(sourceB)
				const makeBasis = (first: THREE.Vector3, second: THREE.Vector3) => {
					const xAxis = first.clone().normalize()
					const yAxis = second.clone().addScaledVector(xAxis, -xAxis.dot(second)).normalize()
					const zAxis = new THREE.Vector3().crossVectors(xAxis, yAxis).normalize()
					return new THREE.Matrix4().makeBasis(xAxis, yAxis, zAxis)
				}
				const sourceBasisInverse = makeBasis(sourceA, sourceB).transpose()
				const rotations: THREE.Quaternion[] = []
				for (const targetA of faces) {
					for (const targetB of faces) {
						if (Math.abs(targetA.localNormal.dot(targetB.localNormal) - sourceDot) > 1e-6) continue
						const matrix = makeBasis(targetA.localNormal, targetB.localNormal).multiply(sourceBasisInverse)
						const candidate = new THREE.Quaternion().setFromRotationMatrix(matrix).normalize()
						if (!this.isColliderSymmetry(resource, candidate)) continue
						if (!rotations.some(rotation => Math.abs(rotation.dot(candidate)) > 1 - 1e-7)) rotations.push(candidate)
					}
				}
				this.symmetryRotations.set(type, rotations)
				return rotations
			}

			private isColliderSymmetry(resource: DiceGeometryResource, rotation: THREE.Quaternion) {
				const vertices = resource.colliderVertices
				for (let sourceIndex = 0; sourceIndex < vertices.length; sourceIndex += 3) {
					tempVector.set(vertices[sourceIndex], vertices[sourceIndex + 1], vertices[sourceIndex + 2]).applyQuaternion(rotation)
					let matched = false
					for (let targetIndex = 0; targetIndex < vertices.length; targetIndex += 3) {
						const dx = tempVector.x - vertices[targetIndex]
						const dy = tempVector.y - vertices[targetIndex + 1]
						const dz = tempVector.z - vertices[targetIndex + 2]
						if (dx * dx + dy * dy + dz * dz < 1e-8) {
							matched = true
							break
						}
					}
					if (!matched) return false
				}
				return true
			}

			private predictOutcomeOffsets(dice: ActiveDie[]) {
				if (!this.world || dice.length === 0) return
				const prediction = RAPIER.World.restoreSnapshot(this.world.takeSnapshot())
				try {
					prediction.timestep = FIXED_STEP
					let steps = 0
					let allSleeping = false
					while (steps < MAX_PREDICTION_STEPS) {
						prediction.step()
						steps += 1
						allSleeping = steps > 30 && dice.every(die => prediction.getRigidBody(die.body.handle).isSleeping())
						if (allSleeping) break
					}
					if (!allSleeping) {
						console.warn('3D 骰子预模拟未在时间上限内全部静止，将在实时刚体休眠时完成最终出目校正')
					}
				for (const die of dice) {
					const predictedBody = prediction.getRigidBody(die.body.handle)
					const rotation = predictedBody.rotation()
					tempQuaternion.set(rotation.x, rotation.y, rotation.z, rotation.w).normalize()
					const rawValue = detectTopFace(die.resource.registryType, tempQuaternion)
					die.visualOffset.copy(this.buildFaceSwapQuaternion(die.resource.registryType, rawValue, die.targetValue))
					tempQuaternionB.copy(tempQuaternion).multiply(die.visualOffset)
					if (detectTopFace(die.resource.registryType, tempQuaternionB) !== die.targetValue) {
						throw new Error(`骰面映射失败：${die.resource.registryType} ${rawValue} -> ${die.targetValue}`)
					}
				}
			} finally {
				prediction.free()
			}
		}

		private tryStartNextThrow() {
			if (!this.world || this.disposed || this.drag || this.activeThrowDice.length > 0) return
			if (this.dice.some(die => !die.settled)) return
			const payload = this.pendingThrows.shift()
			if (payload) this.startThrow(payload)
		}

	  private rebuildBoundaries() {
	    if (!this.world) return
	    const aspect = Math.max(0.6, this.width / this.height)
	    const halfDepth = 4.4
	    const halfWidth = Math.min(9, halfDepth * aspect)
			this.arenaHalfWidth = halfWidth
			this.arenaHalfDepth = halfDepth
			this.boundaryDirty = false
			const specs = [
				{ floor: true, halfExtents: { x: halfWidth, y: 0.18, z: halfDepth }, translation: { x: 0, y: -0.3, z: 0 } },
				{ floor: false, halfExtents: { x: 0.18, y: 4, z: halfDepth }, translation: { x: -halfWidth, y: 3.5, z: 0 } },
				{ floor: false, halfExtents: { x: 0.18, y: 4, z: halfDepth }, translation: { x: halfWidth, y: 3.5, z: 0 } },
				{ floor: false, halfExtents: { x: halfWidth, y: 4, z: 0.18 }, translation: { x: 0, y: 3.5, z: -halfDepth } },
				{ floor: false, halfExtents: { x: halfWidth, y: 4, z: 0.18 }, translation: { x: 0, y: 3.5, z: halfDepth } },
			]
			if (this.boundaries.length === 0) {
				for (const spec of specs) {
					const desc = RAPIER.ColliderDesc.cuboid(spec.halfExtents.x, spec.halfExtents.y, spec.halfExtents.z)
						.setTranslation(spec.translation.x, spec.translation.y, spec.translation.z)
						.setFriction(spec.floor ? 0.58 : 0.04)
						.setRestitution(spec.floor ? 0.18 : this.wallBounce)
						.setFrictionCombineRule(spec.floor ? RAPIER.CoefficientCombineRule.Average : RAPIER.CoefficientCombineRule.Min)
						.setRestitutionCombineRule(spec.floor ? RAPIER.CoefficientCombineRule.Average : RAPIER.CoefficientCombineRule.Max)
					this.boundaries.push(this.world.createCollider(desc))
			}
			return
    }
		for (let index = 0; index < specs.length; index += 1) {
			const collider = this.boundaries[index]
			const spec = specs[index]
				collider.setHalfExtents(spec.halfExtents)
				collider.setTranslation(spec.translation)
				collider.setFriction(spec.floor ? 0.58 : 0.04)
				collider.setRestitution(spec.floor ? 0.18 : this.wallBounce)
				collider.setFrictionCombineRule(spec.floor ? RAPIER.CoefficientCombineRule.Average : RAPIER.CoefficientCombineRule.Min)
				collider.setRestitutionCombineRule(spec.floor ? RAPIER.CoefficientCombineRule.Average : RAPIER.CoefficientCombineRule.Max)
			}
	  }

		private removeOldestSettled(count: number) {
			const targets = this.dice
				.filter(die => die.settled && this.drag?.die !== die)
			.sort((left, right) => left.expiresAt - right.expiresAt)
			.slice(0, Math.max(0, count))
    targets.forEach(die => {
      this.removeDie(die)
      this.dice = this.dice.filter(item => item !== die)
    })
  }

		private removeDie(die: ActiveDie) {
			if (this.drag?.die === die) this.endDrag(false)
			this.scene.remove(die.mesh)
		const materials = Array.isArray(die.mesh.material) ? die.mesh.material : [die.mesh.material]
		materials.forEach(material => material.dispose())
		die.mesh.traverse(object => {
			if (object instanceof THREE.LineSegments) {
				const edgeMaterials = Array.isArray(object.material) ? object.material : [object.material]
				edgeMaterials.forEach(material => material.dispose())
			}
			})
			this.world?.removeRigidBody(die.body)
			this.activeThrowDice = this.activeThrowDice.filter(item => item !== die)
	  }

		private hasContact(die: ActiveDie) {
			if (!this.world) return false
			let contact = false
			this.world.contactPairsWith(die.collider, () => { contact = true })
			return contact
		}

		private ensureAuthoritativeFace(die: ActiveDie, bodyQuaternion: THREE.Quaternion) {
			if (!die.authoritative) return
			tempQuaternionB.copy(bodyQuaternion).multiply(die.visualOffset)
			if (detectTopFace(die.resource.registryType, tempQuaternionB) === die.targetValue) return
			const rawValue = detectTopFace(die.resource.registryType, bodyQuaternion)
			die.visualOffset.copy(this.buildFaceSwapQuaternion(die.resource.registryType, rawValue, die.targetValue))
		}

	  private tick = (now: number) => {
	    if (this.disposed) return
	    const elapsed = Math.min(0.1, Math.max(0, (now - this.lastFrame) / 1000))
	    this.lastFrame = now
	    this.accumulator += elapsed
			let frameSteps = 0
			while (this.world && this.accumulator >= FIXED_STEP && frameSteps < MAX_FRAME_STEPS) {
				this.dice.forEach(die => {
					const position = die.body.translation()
					const rotation = die.body.rotation()
					die.previousPosition.set(position.x, position.y, position.z)
					die.previousQuaternion.set(rotation.x, rotation.y, rotation.z, rotation.w)
				})
				this.updateDragConstraint()
				this.world.step()
				this.accumulator -= FIXED_STEP
				frameSteps += 1
			}
			if (frameSteps === MAX_FRAME_STEPS && this.accumulator >= FIXED_STEP) {
				this.accumulator %= FIXED_STEP
			}
			const interpolation = clamp(this.accumulator / FIXED_STEP, 0, 1)
	    this.dice.forEach(die => {
	      const position = die.body.translation()
	      const rotation = die.body.rotation()
				tempVector.set(position.x, position.y, position.z)
	      die.mesh.position.lerpVectors(die.previousPosition, tempVector, interpolation)
				tempQuaternion.set(rotation.x, rotation.y, rotation.z, rotation.w).normalize()
				tempQuaternionB.copy(die.previousQuaternion).slerp(tempQuaternion, interpolation).multiply(die.visualOffset)
	      die.mesh.quaternion.copy(tempQuaternionB)
				if (this.drag?.die === die) return
			  const linear = die.body.linvel()
			  const angular = die.body.angvel()
				const radius = Math.max(0.05, die.resource.radius * die.scale)
				const speedScale = Math.sqrt(GRAVITY * radius)
				const linearNormalized = Math.hypot(linear.x, linear.y, linear.z) / speedScale
				const angularNormalized = Math.hypot(angular.x, angular.y, angular.z) * Math.sqrt(radius / GRAVITY)
				const rawFace = detectTopFace(die.resource.registryType, tempQuaternion)
				const sameFace = die.stableFace === rawFace
					const stable = sameFace && (die.authoritative
						? die.body.isSleeping()
						: die.body.isSleeping() || (this.hasContact(die) && linearNormalized < 0.035 && angularNormalized < 0.055))
				die.stableFace = rawFace
				die.stableTime = stable ? die.stableTime + elapsed : 0
				die.settled = die.stableTime >= 0.45
					if (die.settled) {
						this.ensureAuthoritativeFace(die, tempQuaternion)
						die.authoritative = false
						if (!die.body.isSleeping()) die.body.sleep()
				}
					})
			if (this.activeThrowDice.length > 0 && this.activeThrowDice.every(die => die.settled)) {
				this.activeThrowDice = []
			}
			const allSettled = this.dice.every(die => die.settled)
			const expired = allSettled && !this.drag
				? this.dice.filter(die => now >= die.expiresAt)
				: []
	    expired.forEach(die => this.removeDie(die))
	    if (expired.length) this.dice = this.dice.filter(die => !expired.includes(die))
			if (this.boundaryDirty && this.activeThrowDice.length === 0 && this.dice.every(die => die.settled)) {
				this.rebuildBoundaries()
			}
			this.tryStartNextThrow()
	    this.renderer.render(this.scene, this.camera)
    this.frame = requestAnimationFrame(this.tick)
  }
}

const seededRandom = (seed: number) => {
  let state = (Math.trunc(seed) || 1) >>> 0
  return () => {
    state = (state * 1664525 + 1013904223) >>> 0
    return state / 0x100000000
  }
}

const resolveDiceAssetURL = (source: string) => {
	if (/^(?:https?:|data:|blob:|\/)/i.test(source)) return source
	return `/api/v1/attachment/${encodeURIComponent(source.replace(/^id:/, ''))}`
}
