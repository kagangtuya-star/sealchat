import { createApp } from 'vue'
import ViewerApp from './ViewerApp.vue'
import './viewer.scss'
import '../assets/tiptap-table.css'

const app = createApp(ViewerApp)
app.mount('#app')
