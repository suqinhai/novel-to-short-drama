<script setup>
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Clapperboard, FolderKanban, Activity, Bot, Menu, X, Bell, CircleUserRound, ClipboardCheck, Images } from 'lucide-vue-next'

const route = useRoute()
const sidebarOpen = ref(false)
const navigation = [
  { label: '项目列表', to: '/projects', icon: FolderKanban, matches: ['/projects'] },
  { label: '项目详情', to: '/projects', icon: Clapperboard, matches: ['/projects/'], detailOnly: true },
  { label: '审核中心', to: '/reviews', icon: ClipboardCheck, matches: ['/reviews'] },
  { label: '媒体资产库', to: '/media-assets', icon: Images, matches: ['/media-assets'] },
  { label: '系统诊断', to: '/diagnostics', icon: Activity, matches: ['/diagnostics'] },
  { label: 'AI 配置', to: '/ai-config', icon: Bot, matches: ['/ai-config'] },
]

const isActive = (item) => {
  if (item.detailOnly) return route.path.startsWith('/projects/')
  if (item.to === '/projects') return route.path === '/projects'
  return item.matches.some((path) => route.path.startsWith(path))
}
const pageTitle = computed(() => route.meta.title || '控制台')
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar" :class="{ open: sidebarOpen }">
      <div class="brand">
        <div class="brand-mark"><Clapperboard :size="21" /></div>
        <div><strong>DRAMA FLOW</strong><span>短剧生产中台</span></div>
      </div>
      <button class="sidebar-close" aria-label="关闭菜单" @click="sidebarOpen = false"><X :size="20" /></button>

      <div class="nav-label">工作台</div>
      <nav class="nav-list">
        <RouterLink v-for="item in navigation" :key="item.label" :to="item.to" class="nav-item" :class="{ active: isActive(item) }" @click="sidebarOpen = false">
          <component :is="item.icon" :size="19" :stroke-width="1.8" />
          <span>{{ item.label }}</span>
          <i v-if="isActive(item)"></i>
        </RouterLink>
      </nav>

      <div class="sidebar-foot">
        <div class="live-dot"></div>
        <div><strong>生产环境</strong><span>short_drama · PostgreSQL</span></div>
      </div>
    </aside>

    <div v-if="sidebarOpen" class="sidebar-scrim" @click="sidebarOpen = false"></div>

    <main class="main-area">
      <header class="topbar">
        <button class="menu-button" aria-label="打开菜单" @click="sidebarOpen = true"><Menu :size="21" /></button>
        <div class="page-heading"><span>{{ route.meta.eyebrow }}</span><h1>{{ pageTitle }}</h1></div>
        <div class="top-actions">
          <button class="icon-button" aria-label="通知"><Bell :size="19" /></button>
          <div class="user-chip"><CircleUserRound :size="28" /><div><strong>制作管理员</strong><span>Administrator</span></div></div>
        </div>
      </header>
      <div class="page-content"><RouterView /></div>
    </main>
  </div>
</template>
