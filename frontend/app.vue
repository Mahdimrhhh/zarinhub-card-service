<template>
  <main class="min-h-screen bg-slate-50 px-4 py-10 text-right font-sans ltr:[direction:rtl]">
    <section class="mx-auto w-full max-w-xl">
      <header class="mb-8">
        <p class="mb-2 text-sm font-semibold text-blue-700">سرویس احراز کارت</p>
        <h1 class="text-3xl font-bold text-slate-900 sm:text-4xl">تبدیل شماره کارت به شبا</h1>
        <p class="mt-3 text-slate-600">شماره کارت ۱۶ رقمی را وارد کنید تا شماره شبا از سرویس داخلی دریافت شود.</p>
      </header>

      <form class="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-7" @submit.prevent="submitCard">
        <label for="card" class="mb-2 block text-sm font-semibold text-slate-800">شماره کارت</label>
        <input
          id="card"
          v-model="card"
          class="h-12 w-full rounded-lg border border-slate-300 bg-white px-4 text-lg tracking-[0.2em] outline-none transition focus:border-blue-600 focus:ring-2 focus:ring-blue-100 ltr:[direction:ltr]"
          type="text"
          inputmode="numeric"
          autocomplete="cc-number"
          maxlength="16"
          placeholder="۱۶ رقم شماره کارت"
          :disabled="loading"
          aria-describedby="card-help"
        />
        <p id="card-help" class="mt-2 text-sm text-slate-500">فقط اعداد و دقیقاً ۱۶ رقم وارد کنید.</p>

        <button
          type="submit"
          class="mt-5 h-12 w-full rounded-lg bg-blue-700 px-5 text-base font-semibold text-white transition hover:bg-blue-800 focus:outline-none focus:ring-4 focus:ring-blue-200 disabled:cursor-not-allowed disabled:bg-slate-300"
          :disabled="loading"
          :aria-busy="loading"
        >
          {{ loading ? 'در حال بررسی...' : 'دریافت شماره شبا' }}
        </button>
      </form>

      <div aria-live="polite" class="mt-6 space-y-4">
        <div v-if="validationError" class="rounded-xl border border-red-200 bg-red-50 p-4 text-red-800">
          <p class="font-semibold">ورودی نامعتبر است</p>
          <p class="mt-1 text-sm">{{ validationError }}</p>
        </div>

        <div v-else-if="serviceError" class="rounded-xl border border-amber-200 bg-amber-50 p-4 text-amber-900">
          <p class="font-semibold">درخواست انجام نشد</p>
          <p class="mt-1 text-sm">{{ serviceError }}</p>
          <p v-if="errorCode" class="mt-2 text-xs text-amber-700">کد خطا: {{ errorCode }}</p>
        </div>

        <div v-if="result" class="rounded-2xl border border-emerald-200 bg-emerald-50 p-5">
          <p class="text-sm font-semibold text-emerald-800">پاسخ سرویس</p>
          <dl class="mt-4 space-y-3">
            <div>
              <dt class="text-sm text-emerald-700">شماره شبا</dt>
              <dd class="mt-1 break-all text-xl font-bold text-emerald-950 ltr:[direction:ltr]">{{ result.iban }}</dd>
            </div>
            <div v-if="result.bank_name">
              <dt class="text-sm text-emerald-700">نام بانک</dt>
              <dd class="mt-1 font-medium text-emerald-950">{{ result.bank_name }}</dd>
            </div>
          </dl>
        </div>
      </div>
    </section>
  </main>
</template>

<script setup>
const config = useRuntimeConfig()
const card = ref('')
const loading = ref(false)
const validationError = ref('')
const serviceError = ref('')
const errorCode = ref('')
const result = ref(null)

const apiBase = computed(() => String(config.public.apiBase || 'http://localhost:8080').replace(/\/$/, ''))

function resetStates() {
  validationError.value = ''
  serviceError.value = ''
  errorCode.value = ''
  result.value = null
}

async function submitCard() {
  resetStates()
  const value = card.value.trim()

  if (!value) {
    validationError.value = 'شماره کارت را وارد کنید.'
    return
  }
  if (!/^\d+$/.test(value)) {
    validationError.value = 'شماره کارت فقط می‌تواند شامل اعداد باشد.'
    return
  }
  if (value.length !== 16) {
    validationError.value = 'شماره کارت باید دقیقاً ۱۶ رقم باشد.'
    return
  }

  loading.value = true
  try {
    const response = await $fetch(`${apiBase.value}/api/v1/card-to-iban`, {
      method: 'POST',
      body: { card: value }
    })
    result.value = response.data || null
  } catch (requestError) {
    const payload = requestError?.data || requestError?.response?._data || {}
    errorCode.value = payload?.error?.code || 'REQUEST_FAILED'
    serviceError.value = payload?.error?.message || 'ارتباط با سرویس برقرار نشد.'
  } finally {
    loading.value = false
  }
}
</script>
