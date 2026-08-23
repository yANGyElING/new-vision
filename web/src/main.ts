import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import './styles/main.css'

if ('serviceWorker' in navigator) {
  navigator.serviceWorker.register('/sw.js')
}

createApp(App).use(router).mount('#app')
