<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { getSignDailyCounts, getSignRecords } from '../api'
import type { SignRecord } from '../types'

const pageSize = 25
const records = ref<SignRecord[]>([])
const loading = ref(false)
const calendarLoading = ref(false)
const error = ref('')
const page = ref(1)
const total = ref(0)
const calendarMonth = ref(new Date())
const dailyCounts = ref<Record<string, number>>({})
const selectedDate = ref('')
// 服务器时区偏移(如 "-04:00"/"+08:00"/"Z")：从接口返回的 sign_time 中推导。
// 日历按服务器本地日期分组，点选某天时也必须用同一时区构造筛选区间，
// 否则浏览器时区与服务器时区不同会导致日历数字与列表条数不一致。
const serverOffset = ref<string | null>(null)
const weekdays = ['日', '一', '二', '三', '四', '五', '六']
const todayDate = formatDateKey(new Date())

const calendarDays = computed(() => monthDays(calendarMonth.value))

function extractServerOffset(value: string): string | null {
  const match = /(Z|[+-]\d{2}:\d{2})$/.exec(value)
  return match ? match[1] : null
}

function serverLocalISO(dateKey: string, hour: number, minute: number, second: number, millisecond: number): string {
  const hh = String(hour).padStart(2, '0')
  const mm = String(minute).padStart(2, '0')
  const ss = String(second).padStart(2, '0')
  const ms = String(millisecond).padStart(3, '0')
  const offset = serverOffset.value ?? ''
  return new Date(`${dateKey}T${hh}:${mm}:${ss}.${ms}${offset}`).toISOString()
}

function formatTime(value: string): string {
  return new Date(value).toLocaleString()
}

function formatMonth(value: Date): string {
  return `${value.getFullYear()}年${value.getMonth() + 1}月`
}

function formatSelectedDate(value: string): string {
  const [year, month, day] = value.split('-')
  return `${year}年${Number(month)}月${Number(day)}日`
}

function formatDateKey(value: Date): string {
  const year = value.getFullYear()
  const month = String(value.getMonth() + 1).padStart(2, '0')
  const day = String(value.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function monthDays(value: Date): Array<string | null> {
  const year = value.getFullYear()
  const month = value.getMonth()
  const firstDay = new Date(year, month, 1).getDay()
  const daysInMonth = new Date(year, month + 1, 0).getDate()
  const days: Array<string | null> = Array(firstDay).fill(null)
  for (let day = 1; day <= daysInMonth; day += 1) {
    days.push(formatDateKey(new Date(year, month, day)))
  }
  return days
}

function dateRangeForDay(value: string): { since: string; until: string } {
  // 用服务器时区构造当天 00:00 ~ 23:59:59.999 的区间，
  // 与后端按服务器本地日期分组的日历保持一致（任意浏览器时区下均一致）。
  return {
    since: serverLocalISO(value, 0, 0, 0, 0),
    until: serverLocalISO(value, 23, 59, 59, 999),
  }
}

function canPrev(): boolean {
  return page.value > 1
}

function canNext(): boolean {
  return page.value * pageSize < total.value || records.value.length === pageSize
}

async function loadCalendar() {
  calendarLoading.value = true
  error.value = ''
  try {
    const year = calendarMonth.value.getFullYear()
    const month = calendarMonth.value.getMonth()
    // 按服务器时区取该月首日 00:00 与末日 23:59:59.999 的区间。
    const firstDay = `${year}-${String(month + 1).padStart(2, '0')}-01`
    const lastDayOfMonth = new Date(year, month + 1, 0).getDate()
    const lastDay = `${year}-${String(month + 1).padStart(2, '0')}-${String(lastDayOfMonth).padStart(2, '0')}`
    const since = serverLocalISO(firstDay, 0, 0, 0, 0)
    const until = serverLocalISO(lastDay, 23, 59, 59, 999)
    const response = await getSignDailyCounts({ since, until })
    dailyCounts.value = Object.fromEntries(response.items.map((item) => [item.date, item.count]))
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    calendarLoading.value = false
  }
}

async function loadRecords(nextPage = page.value) {
  loading.value = true
  error.value = ''
  try {
    const safePage = Math.max(1, nextPage)
    const response = selectedDate.value
      ? await getSignRecords(pageSize, (safePage - 1) * pageSize, dateRangeForDay(selectedDate.value))
      : await getSignRecords(pageSize, (safePage - 1) * pageSize)
    records.value = response.items
    total.value = response.total ?? response.offset + response.items.length
    page.value = safePage
    // 从返回记录推导服务器时区偏移；首次获知后刷新日历（保证日历区间与列表筛选同钟）。
    if (serverOffset.value === null && records.value.length > 0 && records.value[0].sign_time) {
      serverOffset.value = extractServerOffset(records.value[0].sign_time)
      loadCalendar()
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

function previousMonth() {
  calendarMonth.value = new Date(calendarMonth.value.getFullYear(), calendarMonth.value.getMonth() - 1, 1)
}

function nextMonth() {
  calendarMonth.value = new Date(calendarMonth.value.getFullYear(), calendarMonth.value.getMonth() + 1, 1)
}

function selectDate(value: string) {
  selectedDate.value = selectedDate.value === value ? '' : value
  loadRecords(1)
}

function clearSelectedDate() {
  selectedDate.value = ''
  loadRecords(1)
}

watch(calendarMonth, () => loadCalendar())

onMounted(() => {
  loadCalendar()
  loadRecords()
})
</script>

<template>
  <section class="admin-dashboard">
    <div class="panel admin-status-panel signed-calendar-panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Calendar</p>
          <h2>签到日历</h2>
        </div>
        <span class="counter">{{ calendarLoading ? '加载中...' : formatMonth(calendarMonth) }}</span>
      </div>

      <div class="signed-calendar-toolbar">
        <button class="admin-button ghost" :disabled="calendarLoading" @click="previousMonth">上一月</button>
        <strong>{{ formatMonth(calendarMonth) }}</strong>
        <button class="admin-button ghost" :disabled="calendarLoading" @click="nextMonth">下一月</button>
      </div>

      <div class="signed-calendar-grid">
        <div v-for="weekday in weekdays" :key="weekday" class="signed-calendar-weekday">{{ weekday }}</div>
        <template v-for="(day, index) in calendarDays" :key="day || `empty-${index}`">
          <div v-if="!day" class="signed-calendar-day empty" aria-hidden="true"></div>
          <button
            v-else
            type="button"
            class="signed-calendar-day"
            :class="{ selected: selectedDate === day, today: todayDate === day, 'has-signs': dailyCounts[day] }"
            @click="selectDate(day)"
          >
            <span class="signed-calendar-date">{{ Number(day.slice(8, 10)) }}</span>
            <span v-if="dailyCounts[day]" class="signed-calendar-count">{{ dailyCounts[day] }} 人</span>
          </button>
        </template>
      </div>

      <div v-if="selectedDate" class="signed-filter-info">
        <span>当前筛选：{{ formatSelectedDate(selectedDate) }}</span>
        <button class="admin-button ghost" :disabled="loading" @click="clearSelectedDate">清除筛选</button>
      </div>
    </div>

    <div class="panel admin-status-panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Signed</p>
          <h2>签到用户</h2>
        </div>
        <span class="counter">共 {{ total }} 条签到记录</span>
      </div>

      <p v-if="error" class="error">{{ error }}</p>
      <div class="node-table-wrap">
        <table class="node-table">
          <thead>
            <tr>
              <th>节点 ID</th>
              <th>Long Name</th>
              <th>Short Name</th>
              <th>签到文本</th>
              <th>签到时间</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="record in records" :key="record.id">
              <td>{{ record.node_id }}</td>
              <td>{{ record.long_name || '-' }}</td>
              <td>{{ record.short_name || '-' }}</td>
              <td>{{ record.sign_text }}</td>
              <td>{{ formatTime(record.sign_time) }}</td>
            </tr>
          </tbody>
        </table>
        <div v-if="records.length === 0" class="empty">{{ loading ? '加载中...' : '暂无签到记录' }}</div>
      </div>

      <div class="pagination">
        <button class="admin-button" :disabled="!canPrev() || loading" @click="loadRecords(page - 1)">上一页</button>
        <span>第 {{ page }} 页</span>
        <button class="admin-button" :disabled="!canNext() || loading" @click="loadRecords(page + 1)">下一页</button>
      </div>
    </div>
  </section>
</template>
