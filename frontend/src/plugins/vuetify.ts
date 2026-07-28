/**
 * Vuetify 局部插件：仅供 ModelStatusPanel 子树使用，不承担全局应用壳。
 */
import 'vuetify/styles'
import '@mdi/font/css/materialdesignicons.css'
import { createVuetify } from 'vuetify'
import { VAlert } from 'vuetify/components/VAlert'
import { VCard, VCardText, VCardTitle } from 'vuetify/components/VCard'
import { VChip } from 'vuetify/components/VChip'
import { VChipGroup } from 'vuetify/components/VChipGroup'
import { VSkeletonLoader } from 'vuetify/components/VSkeletonLoader'
import { aliases, mdi } from 'vuetify/iconsets/mdi'

export const vuetify = createVuetify({
  components: {
    VAlert,
    VCard,
    VCardText,
    VCardTitle,
    VChip,
    VChipGroup,
    VSkeletonLoader,
  },
  icons: {
    defaultSet: 'mdi',
    aliases,
    sets: { mdi },
  },
  theme: {
    defaultTheme: 'light',
  },
})

export default vuetify
