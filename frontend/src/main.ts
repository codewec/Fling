import { mount } from 'svelte'
import App from './App.svelte'
import { startRouter } from './lib/router'
import './app.css'

startRouter()

const app = mount(App, {
  target: document.getElementById('app')!,
})

export default app
