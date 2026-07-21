import { createRouter, createWebHistory } from 'vue-router'
import ProjectsView from '../views/ProjectsView.vue'
import ProjectDetailView from '../views/ProjectDetailView.vue'
import NewProjectView from '../views/NewProjectView.vue'
import DiagnosticsView from '../views/DiagnosticsView.vue'
import AIConfigView from '../views/AIConfigView.vue'
import ReviewsView from '../views/ReviewsView.vue'
import MediaAssetsView from '../views/MediaAssetsView.vue'
import SourceWorksView from '../views/SourceWorksView.vue'
import SourceWorkDetailView from '../views/SourceWorkDetailView.vue'
import SourceVersionView from '../views/SourceVersionView.vue'
import AdaptationScopeView from '../views/AdaptationScopeView.vue'
import ImpactAnalysisView from '../views/ImpactAnalysisView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/projects' },
    { path: '/projects', name: 'projects', component: ProjectsView, meta: { title: '项目列表', eyebrow: 'PRODUCTION OVERVIEW' } },
    { path: '/projects/new', name: 'project-new', component: NewProjectView, meta: { title: '新建项目', eyebrow: 'CREATE PRODUCTION' } },
    { path: '/projects/:projectId', name: 'project-detail', component: ProjectDetailView, meta: { title: '项目详情', eyebrow: 'PROJECT WORKSPACE' } },
    { path: '/projects/:projectId/adaptation-scope', name: 'project-adaptation-scope', component: AdaptationScopeView, meta: { title: '改编范围', eyebrow: 'ADAPTATION SPEC' } },
    { path: '/projects/:projectId/impact', name: 'project-impact', component: ImpactAnalysisView, meta: { title: '修订影响分析', eyebrow: 'SOURCE IMPACT' } },
    { path: '/adaptations/new', name: 'adaptation-new', component: AdaptationScopeView, meta: { title: '新建改编项目', eyebrow: 'ADAPTATION SPEC' } },
    { path: '/library', name: 'source-works', component: SourceWorksView, meta: { title: '原著资料库', eyebrow: 'SOURCE LIBRARY' } },
    { path: '/library/:workId', name: 'source-work-detail', component: SourceWorkDetailView, meta: { title: '作品版本', eyebrow: 'SOURCE LIBRARY' } },
    { path: '/library/versions/:versionId', name: 'source-version-detail', component: SourceVersionView, meta: { title: '章节管理', eyebrow: 'SOURCE VERSION' } },
    { path: '/reviews', name: 'reviews', component: ReviewsView, meta: { title: '审核中心', eyebrow: 'REVIEW OPERATIONS' } },
    { path: '/media-assets', name: 'media-assets', component: MediaAssetsView, meta: { title: '媒体资产库', eyebrow: 'MEDIA LIBRARY' } },
    { path: '/diagnostics', name: 'diagnostics', component: DiagnosticsView, meta: { title: '系统诊断', eyebrow: 'SYSTEM HEALTH' } },
    { path: '/ai-config', name: 'ai-config', component: AIConfigView, meta: { title: 'AI 配置', eyebrow: 'MODEL & PROVIDER' } },
  ],
})

router.afterEach((to) => {
  document.title = `${to.meta.title || '控制台'} · 短剧生产 CMS`
})

export default router
