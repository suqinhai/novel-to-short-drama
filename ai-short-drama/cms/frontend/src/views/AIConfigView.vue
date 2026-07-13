<script setup>
import { onMounted, ref } from 'vue'
import { Bot, BrainCircuit, Image, Video, AudioLines, Send, ShieldCheck, RefreshCw, KeyRound } from 'lucide-vue-next'
import { api } from '../services/api'

const data = ref(null)
const loading = ref(true)
const error = ref('')
const providerIcons = { 图片生成: Image, 视频生成: Video, 语音合成: AudioLines, 发布渠道: Send }

async function load() {
  loading.value = true
  error.value = ''
  try { data.value = await api.getAIConfig() }
  catch (err) { error.value = err.message }
  finally { loading.value = false }
}
onMounted(load)
</script>

<template>
  <section class="view-stack">
    <div class="hero-row"><div><h2>AI 模型与供应商</h2><p>查看当前 n8n 生产链路使用的模型别名和媒体供应商。</p></div><button class="button button-secondary" :disabled="loading" @click="load"><RefreshCw :size="16" :class="{ spin: loading }" />重新读取</button></div>
    <div class="config-notice"><ShieldCheck :size="20" /><div><strong>安全的只读视图</strong><p>配置继承自现有环境变量。CMS 不展示密钥，也不会修改 n8n 工作流。</p></div><span>{{ data?.source || '环境变量' }}</span></div>
    <div v-if="error" class="error-banner large">{{ error }} <button @click="load">重试</button></div>
    <div v-else-if="loading" class="config-loading"><span></span><span></span></div>
    <template v-else-if="data">
      <article class="panel padded">
        <div class="section-title"><div><span>LANGUAGE MODELS</span><h3>文本生成模型</h3></div><div class="section-icon"><BrainCircuit :size="19" /></div></div>
        <div class="model-list"><div v-for="item in data.text_models" :key="item.env_key" class="model-row"><div class="model-avatar"><Bot :size="17" /></div><div><strong>{{ item.stage }}</strong><span>{{ item.env_key }}</span></div><code>{{ item.model }}</code></div></div>
      </article>
      <article class="panel padded">
        <div class="section-title"><div><span>MEDIA PROVIDERS</span><h3>媒体与发布供应商</h3></div><div class="section-icon"><KeyRound :size="19" /></div></div>
        <div class="provider-grid"><div v-for="item in data.providers" :key="item.name" class="provider-card"><div class="provider-icon"><component :is="providerIcons[item.name]" :size="21" /></div><div class="provider-title"><span>{{ item.name }}</span><i :class="{ configured: item.credential_configured }">{{ item.credential_configured ? '凭证已配置' : '无需或未配置凭证' }}</i></div><dl><div><dt>供应商</dt><dd>{{ item.provider }}</dd></div><div><dt>模型 / 渠道</dt><dd>{{ item.model }}</dd></div></dl></div></div>
      </article>
    </template>
  </section>
</template>
