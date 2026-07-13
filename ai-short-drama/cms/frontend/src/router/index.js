import { createRouter, createWebHistory } from 'vue-router'
import ProjectsView from '../views/ProjectsView.vue'
import ProjectDetailView from '../views/ProjectDetailView.vue'
import NewProjectView from '../views/NewProjectView.vue'
import DiagnosticsView from '../views/DiagnosticsView.vue'
import AIConfigView from '../views/AIConfigView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/projects' },
    { path: '/projects', name: 'projects', component: ProjectsView, meta: { title: '项目列表', eyebrow: 'PRODUCTION OVERVIEW' } },
    { path: '/projects/new', name: 'project-new', component: NewProjectView, meta: { title: '新建项目', eyebrow: 'CREATE PRODUCTION' } },
    { path: '/projects/:projectId', name: 'project-detail', component: ProjectDetailView, meta: { title: '项目详情', eyebrow: 'PROJECT WORKSPACE' } },
    { path: '/diagnostics', name: 'diagnostics', component: DiagnosticsView, meta: { title: '系统诊断', eyebrow: 'SYSTEM HEALTH' } },
    { path: '/ai-config', name: 'ai-config', component: AIConfigView, meta: { title: 'AI 配置', eyebrow: 'MODEL & PROVIDER' } },
  ],
})

router.afterEach((to) => {
  document.title = `${to.meta.title || '控制台'} · 短剧生产 CMS`
})

export default router
