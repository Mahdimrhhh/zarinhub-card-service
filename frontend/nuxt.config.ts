export default defineNuxtConfig({
  modules: ['@nuxtjs/tailwindcss'],
  css: ['~/assets/css/main.css'],
  runtimeConfig: {
    public: {
      apiBase: process.env.NUXT_PUBLIC_API_BASE || 'http://localhost:8080'
    }
  },
  devtools: { enabled: false },
  compatibilityDate: '2025-07-15',
  app: {
    head: {
      title: 'تبدیل شماره کارت به شبا',
      meta: [
        { name: 'description', content: 'درخواست تبدیل شماره کارت به شماره شبا' }
      ]
    }
  }
})
