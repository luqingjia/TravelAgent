import { createApp } from 'vue'
import { createPinia } from 'pinia'
import Antd from 'ant-design-vue'
import 'ant-design-vue/dist/reset.css'

import App from './App.vue'
import router from './router'
import vuetify from './plugins/vuetify'
// Element Plus 自定义 namespace tael 的编译样式（与 ElConfigProvider.namespace 对齐）
import './styles/element/index.scss'

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(Antd)
app.use(vuetify)

app.mount('#app')
