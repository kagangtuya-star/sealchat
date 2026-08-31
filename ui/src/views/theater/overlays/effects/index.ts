import { registerSceneOverlayEffect, validateSceneOverlayEffectRegistry } from '../scene-overlay-registry'
import customMedia from './custom-media'
import ashfall from './weather/ashfall'
import blizzard from './weather/blizzard'
import drizzle from './weather/drizzle'
import fog from './weather/fog'
import hail from './weather/hail'
import mist from './weather/mist'
import rainHeavy from './weather/rain-heavy'
import rainLight from './weather/rain-light'
import rainMedium from './weather/rain-medium'
import sandstorm from './weather/sandstorm'
import sleet from './weather/sleet'
import snow from './weather/snow'
import snowLight from './weather/snow-light'
import storm from './weather/storm'
import autumnLeaves from './environment/autumn-leaves'
import bubbles from './environment/bubbles'
import cinders from './environment/cinders'
import dust from './environment/dust'
import embers from './environment/embers'
import feathers from './environment/feathers'
import fireflies from './environment/fireflies'
import leaves from './environment/leaves'
import petals from './environment/petals'
import pollen from './environment/pollen'
import smoke from './environment/smoke'
import soot from './environment/soot'
import sparks from './environment/sparks'
import spores from './environment/spores'
import steam from './environment/steam'
import arcane from './magic/arcane'
import blood from './magic/blood'
import frost from './magic/frost'
import holy from './magic/holy'
import motes from './magic/motes'
import nature from './magic/nature'
import necrotic from './magic/necrotic'
import poison from './magic/poison'
import psychic from './magic/psychic'
import voidEffect from './magic/void'
import bloodMoon from './lighting/blood-moon'
import candlelight from './lighting/candlelight'
import cold from './lighting/cold'
import colorWash from './lighting/color-wash'
import dawn from './lighting/dawn'
import dream from './lighting/dream'
import dusk from './lighting/dusk'
import eclipse from './lighting/eclipse'
import firelight from './lighting/firelight'
import moonless from './lighting/moonless'
import moonlight from './lighting/moonlight'
import night from './lighting/night'
import overcast from './lighting/overcast'
import sunrise from './lighting/sunrise'
import sunset from './lighting/sunset'
import toxic from './lighting/toxic'
import twilight from './lighting/twilight'
import underwater from './lighting/underwater'
import warmDay from './lighting/warm-day'
import blackout from './special/blackout'
import flash from './special/flash'
import lightning from './special/lightning'
import pulseMagic from './special/pulse-magic'
import pulseRed from './special/pulse-red'

export const builtInSceneOverlayEffects = [
  customMedia,
  rainLight, rainMedium, rainHeavy, drizzle, storm, snowLight, snow, blizzard, sleet, hail, mist, fog, sandstorm, ashfall,
  dust, embers, sparks, fireflies, leaves, autumnLeaves, petals, feathers, pollen, spores, bubbles, smoke, soot, steam, cinders,
  motes, arcane, holy, necrotic, voidEffect, blood, poison, psychic, frost, nature,
  dawn, sunrise, warmDay, overcast, dusk, sunset, twilight, night, moonlight, moonless, eclipse, firelight, candlelight, underwater, dream, bloodMoon, toxic, cold, colorWash,
  lightning, flash, blackout, pulseRed, pulseMagic,
]

let registered = false

export const registerBuiltInSceneOverlayEffects = () => {
  if (registered) return
  registered = true
  builtInSceneOverlayEffects.forEach(registerSceneOverlayEffect)
  if (import.meta.env.DEV) {
    const issues = validateSceneOverlayEffectRegistry()
    if (issues.length) throw new Error(issues.join('\n'))
  }
}
