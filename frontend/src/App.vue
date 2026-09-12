<script setup>
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import { Events } from "@wailsio/runtime"
import {
  GetSystemInfo,
  FullActivation,
  CheckStatus,
  CheckDetails,
  RelaunchAsAdmin,
  GetHardwareSpecs,
  PrintSpecsToConsole,
  PingKMSServer,
  PingKMSServers,
  ResetKey,
  ClearKMSServer,
  Rearm,
  RestartSPPService
} from '../bindings/Activator/activatorservice.js'

// Ключ продукта по умолчанию (Win 10/11 Pro)
const productKey = ref('W269N-WFGWX-YVC9B-4J6C9-T83GX')

// Список проверенных KMS серверов для быстрого выбора кликом (без выпадающих списков)
const kmsPresets = [
  'kms8.msguides.com',
  'kms.digiboy.ir',
  'kms.lotro.cc',
  'kms.loli.best'
]

const currentServer = ref(kmsPresets[0])

const sysInfo = ref({
  osName: 'Загрузка...',
  displayVersion: '',
  buildNumber: '',
  editionId: '',
  arch: '',
  isAdmin: false,
  defaultKey: 'W269N-WFGWX-YVC9B-4J6C9-T83GX',
  defaultServer: 'kms8.msguides.com'
})

const logs = ref([])
const isBusy = ref(false)
const currentAction = ref('')
const terminalRef = ref(null)
const matrixCanvasRef = ref(null)
let matrixAnimId = null
let resizeObserver = null
const copySuccess = ref(false)

// Состояние доступности (пинг) KMS-серверов
const pingMap = ref({})
const isPingingAll = ref(false)

// Состояние панели устранения неполадок
const showTroubleshoot = ref(false)

// Состояние модального окна характеристик ПК
const showSpecsModal = ref(false)
const specs = ref(null)
const isLoadingSpecs = ref(false)
const copySpecsSuccess = ref(false)

async function openSpecsModal() {
  showSpecsModal.value = true
  if (!specs.value) {
    await refreshSpecs()
  }
}

async function refreshSpecs() {
  isLoadingSpecs.value = true
  try {
    const res = await GetHardwareSpecs()
    if (res) {
      specs.value = res
    }
  } catch (err) {
    appendLog('✖ Ошибка загрузки характеристик: ' + err)
  } finally {
    isLoadingSpecs.value = false
  }
}

function closeSpecsModal() {
  showSpecsModal.value = false
}

async function handlePrintSpecsToConsole() {
  try {
    await PrintSpecsToConsole()
    showSpecsModal.value = false
  } catch (err) {
    appendLog('✖ Ошибка вывода характеристик: ' + err)
  }
}

async function copySpecsText() {
  if (!specs.value) return
  const s = specs.value
  const gpusText = s.gpus && s.gpus.length ? s.gpus.join(', ') : 'Стандартный видеоадаптер'
  let disksText = ''
  if (s.disks && s.disks.length) {
    disksText = s.disks.map(d => `${d.letter} всего ${d.totalGb} (свободно ${d.freeGb})`).join('\n')
  }
  const text = `=== Характеристики ПК ===
Компьютер: ${s.computerName} (Пользователь: ${s.userName})
ОС: ${s.osName} ${s.osVersion} [Сборка ${s.osBuild}, ${s.arch}]
Время работы: ${s.uptime}
Процессор: ${s.cpuName} (${s.cpuCores} потоков)
Оперативная память: ${s.totalRam} (Занято: ${s.usedRam} - ${s.ramPercent}%, Свободно: ${s.availRam})
Видеокарта: ${gpusText}
Материнская плата: ${s.motherboard} (BIOS: ${s.biosInfo})
Накопители:
${disksText}`

  try {
    await navigator.clipboard.writeText(text)
    copySpecsSuccess.value = true
    setTimeout(() => { copySpecsSuccess.value = false }, 2000)
  } catch (err) {
    appendLog('✖ Ошибка копирования: ' + err)
  }
}

function resetKey() {
  productKey.value = 'W269N-WFGWX-YVC9B-4J6C9-T83GX'
  appendLog('Ключ сброшен на значение по умолчанию.')
}

function appendLog(line) {
  if (!line) return
  const time = new Date().toLocaleTimeString()
  logs.value.push({ time, text: line })
  nextTick(() => {
    if (terminalRef.value) {
      terminalRef.value.scrollTop = terminalRef.value.scrollHeight
    }
  })
}

function clearLogs() {
  logs.value = []
}

async function copyLogs() {
  const text = logs.value.map(l => `[${l.time}] ${l.text}`).join('\n')
  try {
    await navigator.clipboard.writeText(text)
    copySuccess.value = true
    setTimeout(() => { copySuccess.value = false }, 2000)
  } catch (err) {
    appendLog('✖ Ошибка копирования в буфер: ' + err)
  }
}

async function loadSystemInfo() {
  try {
    const res = await GetSystemInfo()
    if (res) {
      sysInfo.value = res
      appendLog(`[SYSTEM] Система: ${res.osName} ${res.displayVersion} (${res.arch})`)
      if (res.isAdmin) {
        appendLog('[SYSTEM] Приложение запущено с правами Администратора.')
      } else {
        appendLog('[WARNING] Запущено БЕЗ прав Администратора. Для slmgr рекомендуются права админа.')
      }
    }
  } catch (err) {
    appendLog('✖ Ошибка получения информации о системе: ' + err)
  }
}

async function handleRelaunchAdmin() {
  appendLog('Запрос перезапуска с правами Администратора (UAC)...')
  try {
    await RelaunchAsAdmin()
  } catch (err) {
    appendLog('✖ Ошибка перезапуска: ' + err)
  }
}

async function runAction(name, fn) {
  if (isBusy.value) return
  isBusy.value = true
  currentAction.value = name
  try {
    await fn()
  } catch (err) {
    appendLog(`✖ Ошибка [${name}]: ` + (err?.message || err))
  } finally {
    isBusy.value = false
    currentAction.value = ''
  }
}

// 1-клик полная активация
function doFullActivation() {
  runAction('Активация', async () => {
    await FullActivation(productKey.value, currentServer.value)
  })
}

// Проверка срока лицензии /xpr
function doCheckStatus() {
  runAction('Срок лицензии', async () => {
    await CheckStatus()
  })
}

// Сведения о лицензии /dli
function doCheckDetails() {
  runAction('Сведения о лицензии', async () => {
    await CheckDetails()
  })
}

// Проверка доступности KMS серверов
async function pingAllServers() {
  if (isPingingAll.value) return
  isPingingAll.value = true

  const list = [...kmsPresets]
  const cur = currentServer.value?.trim()
  if (cur && !list.includes(cur)) {
    list.push(cur)
  }

  list.forEach(srv => {
    pingMap.value[srv] = { ...(pingMap.value[srv] || {}), loading: true }
  })

  try {
    const results = await PingKMSServers(list)
    if (results && Array.isArray(results)) {
      results.forEach(res => {
        pingMap.value[res.server] = {
          online: res.online,
          latency: res.latency,
          loading: false
        }
      })
    }
  } catch (err) {
    console.error('Ошибка проверки KMS серверов:', err)
  } finally {
    isPingingAll.value = false
  }
}

async function pingCurrentServer() {
  const srv = currentServer.value?.trim()
  if (!srv) return
  pingMap.value[srv] = { ...(pingMap.value[srv] || {}), loading: true }

  try {
    const res = await PingKMSServer(srv)
    if (res) {
      pingMap.value[srv] = {
        online: res.online,
        latency: res.latency,
        loading: false
      }
    }
  } catch (err) {
    pingMap.value[srv] = {
      online: false,
      latency: 0,
      loading: false
    }
  }
}

function selectServer(srv) {
  currentServer.value = srv
  appendLog(`Выбран KMS сервер: ${srv}`)
  if (!pingMap.value[srv] || pingMap.value[srv].latency === undefined) {
    pingCurrentServer()
  }
}

// Функции устранения неполадок
function doUninstallKey() {
  runAction('Удаление ключа', async () => {
    await ResetKey()
  })
}

function doClearKMSServer() {
  runAction('Очистка KMS', async () => {
    await ClearKMSServer()
  })
}

function doRearm() {
  runAction('Сброс таймера (Rearm)', async () => {
    await Rearm()
  })
}

function doRestartSPP() {
  runAction('Перезапуск sppsvc', async () => {
    await RestartSPPService()
  })
}

// Эффект фонового матричного дождя в терминале
function initMatrixRain() {
  const canvas = matrixCanvasRef.value
  if (!canvas) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  const parent = canvas.parentElement
  if (!parent) return

  let width = (canvas.width = parent.clientWidth)
  let height = (canvas.height = parent.clientHeight)

  // Набор символов: случайные цифры и буквы, как в матрице
  const chars = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789'
  const fontSize = 12
  let columns = Math.max(1, Math.floor(width / fontSize))
  let drops = []

  function resetDrops() {
    columns = Math.max(1, Math.floor(width / fontSize))
    drops = []
    for (let i = 0; i < columns; i++) {
      drops[i] = Math.floor(Math.random() * -35)
    }
  }
  resetDrops()

  let lastTime = 0
  const fps = 25
  const interval = 1000 / fps

  function draw(currentTime) {
    matrixAnimId = requestAnimationFrame(draw)

    if (!currentTime) currentTime = performance.now()
    const delta = currentTime - lastTime
    if (delta < interval) return
    lastTime = currentTime - (delta % interval)

    // Плавный затухающий шлейф цветом фона терминала
    ctx.fillStyle = 'rgba(9, 13, 19, 0.12)'
    ctx.fillRect(0, 0, width, height)

    ctx.font = `${fontSize}px monospace`

    for (let i = 0; i < drops.length; i++) {
      const char = chars[Math.floor(Math.random() * chars.length)]
      const x = i * fontSize
      const y = drops[i] * fontSize

      if (y > 0 && y < height + fontSize * 2) {
        // Лидирующий символ периодически подсвечивается светлым бликом
        if (Math.random() > 0.88) {
          ctx.fillStyle = '#a7f3d0'
        } else {
          ctx.fillStyle = '#10b981'
        }
        ctx.fillText(char, x, y)
      }

      if (y > height && Math.random() > 0.975) {
        drops[i] = 0
      }
      drops[i]++
    }
  }

  matrixAnimId = requestAnimationFrame(draw)

  if (window.ResizeObserver) {
    resizeObserver = new ResizeObserver(() => {
      if (!canvas || !parent) return
      width = canvas.width = parent.clientWidth
      height = canvas.height = parent.clientHeight
      resetDrops()
      ctx.fillStyle = '#090d13'
      ctx.fillRect(0, 0, width, height)
    })
    resizeObserver.observe(parent)
  }
}

onMounted(() => {
  // Слушаем события логов из Go бэкенда
  Events.On('cmd-output', (event) => {
    if (event && event.data) {
      appendLog(event.data)
    }
  })

  appendLog('════════════ Windows Activator (slmgr + KMS) ════════════')
  appendLog('Готов к работе. Ключ: W269N-WFGWX-YVC9B-4J6C9-T83GX')
  loadSystemInfo()

  nextTick(() => {
    initMatrixRain()
  })

  // Фоновая проверка доступности KMS-серверов
  pingAllServers()
})

onUnmounted(() => {
  if (matrixAnimId) {
    cancelAnimationFrame(matrixAnimId)
  }
  if (resizeObserver) {
    resizeObserver.disconnect()
  }
})
</script>

<template>
  <div class="app-layout">
    <!-- Основная рабочая область -->
    <main class="main-content">
      <!-- Левая колонка: Настройки ключа, KMS и кнопки действий -->
      <section class="controls-column">
        <!-- Ключ продукта -->
        <div class="card compact-card">
          <div class="form-group">
            <label class="form-label">Ключ продукта (GVLK для Windows 10/11 Pro):</label>
            <div class="key-input-wrapper">
              <input
                v-model="productKey"
                type="text"
                spellcheck="false"
                class="form-input key-input"
                placeholder="XXXXX-XXXXX-XXXXX-XXXXX-XXXXX"
              />
              <button
                class="btn-icon"
                @click="resetKey"
                title="Сбросить ключ на значение по умолчанию"
              >
                ↺
              </button>
            </div>
          </div>

          <!-- KMS Сервер: кликабельные чипы + поле ввода -->
          <div class="form-group mt-8">
            <div class="label-with-action">
              <label class="form-label">KMS Сервер активации:</label>
              <button
                class="btn-text-action"
                type="button"
                @click="pingAllServers"
                :disabled="isPingingAll"
                title="Проверить доступность всех KMS-серверов"
              >
                <span :class="{ 'spin-anim': isPingingAll }">🔄</span> Пинг
              </button>
            </div>

            <div class="chips-container">
              <button
                v-for="srv in kmsPresets"
                :key="srv"
                class="chip"
                :class="{ active: currentServer === srv }"
                @click="selectServer(srv)"
                type="button"
              >
                <span class="chip-label">{{ srv }}</span>
                <span
                  v-if="pingMap[srv]?.loading"
                  class="ping-dot ping-loading"
                  title="Проверка..."
                ></span>
                <span
                  v-else-if="pingMap[srv]?.online"
                  class="ping-badge ping-online"
                  :title="`Онлайн: ${pingMap[srv].latency} мс`"
                >
                  <span class="ping-dot dot-green"></span>{{ pingMap[srv].latency }}ms
                </span>
                <span
                  v-else-if="pingMap[srv]?.online === false"
                  class="ping-badge ping-offline"
                  title="Сервер не отвечает"
                >
                  <span class="ping-dot dot-red"></span>off
                </span>
              </button>
            </div>
            <div class="server-input-wrapper">
              <input
                v-model="currentServer"
                type="text"
                class="form-input server-input"
                placeholder="Адрес KMS сервера"
                @blur="pingCurrentServer"
                @keyup.enter="pingCurrentServer"
              />
              <button
                class="btn-icon"
                type="button"
                @click="pingCurrentServer"
                :disabled="pingMap[currentServer]?.loading"
                title="Проверить доступность текущего сервера"
              >
                <span v-if="pingMap[currentServer]?.loading" class="spinner-tiny"></span>
                <span v-else>📶</span>
              </button>
            </div>
          </div>
        </div>

        <!-- Кнопки действий: 1-клик Активация + Срок лицензии и Сведения -->
        <div class="card action-card">
          <button
            class="btn-primary"
            :disabled="isBusy || !productKey.trim()"
            @click="doFullActivation"
          >
            <span v-if="isBusy && currentAction === 'Активация'" class="spinner"></span>
            <span v-else class="btn-icon-large">🚀</span>
            <span class="btn-text">
              <strong>Активировать Windows</strong>
              <small>Установка ключа, KMS и активация в 1 клик</small>
            </span>
          </button>

          <!-- Оставлены только Срок лицензии и Сведения -->
          <div class="btn-grid-two">
            <button
              class="btn-secondary"
              :disabled="isBusy"
              @click="doCheckStatus"
              title="Выполняет slmgr /xpr (срок действия лицензии)"
            >
              <span>ℹ️ Срок лицензии</span>
            </button>

            <button
              class="btn-secondary"
              :disabled="isBusy"
              @click="doCheckDetails"
              title="Выполняет slmgr /dli (сведения о лицензии)"
            >
              <span>📄 Сведения</span>
            </button>
          </div>

          <!-- Кнопка Характеристики ПК на всю ширину карточки -->
          <button
            class="btn-secondary btn-specs-full"
            @click="openSpecsModal"
            title="Посмотреть подробные характеристики ПК (CPU, RAM, GPU, Диски)"
          >
            <span>💻 Характеристики компьютера</span>
          </button>

          <!-- Индикатор статуса Администратора под Характеристиками ПК -->
          <div class="admin-status-box">
            <div v-if="sysInfo.isAdmin" class="badge badge-admin">
              <span class="status-dot dot-green"></span>
              <span>Администратор (UAC OK)</span>
            </div>
            <div v-else class="badge-group">
              <div class="badge badge-warn">
                <span class="status-dot dot-amber"></span>
                <span>Без прав админа</span>
              </div>
              <button class="btn-elevate" @click="handleRelaunchAdmin" title="Перезапустить с правами администратора">
                Перезапустить (UAC)
              </button>
            </div>
          </div>

          <!-- Раздел Устранение неполадок / Сброс -->
          <div class="troubleshoot-box">
            <button
              class="btn-troubleshoot-toggle"
              type="button"
              @click="showTroubleshoot = !showTroubleshoot"
              :class="{ active: showTroubleshoot }"
            >
              <span class="troubleshoot-toggle-title">🛠️ Устранение неполадок</span>
              <span class="toggle-arrow">{{ showTroubleshoot ? '▲' : '▼' }}</span>
            </button>

            <div v-if="showTroubleshoot" class="troubleshoot-grid">
              <button
                class="btn-trouble btn-danger"
                :disabled="isBusy"
                @click="doUninstallKey"
                title="Удаляет ключ из системы и реестра (slmgr /upk + /cpky)"
              >
                <span>🗑️ Удалить ключ</span>
              </button>
              <button
                class="btn-trouble"
                :disabled="isBusy"
                @click="doClearKMSServer"
                title="Сбрасывает настроенный адрес KMS-сервера (slmgr /ckms)"
              >
                <span>🧹 Очистить KMS</span>
              </button>
              <button
                class="btn-trouble"
                :disabled="isBusy"
                @click="doRearm"
                title="Сбрасывает таймер льготного периода активации (slmgr /rearm)"
              >
                <span>🔄 Rearm (Сброс)</span>
              </button>
              <button
                class="btn-trouble"
                :disabled="isBusy"
                @click="doRestartSPP"
                title="Перезапускает службу защиты программного обеспечения Windows (sppsvc)"
              >
                <span>⚙️ Служба sppsvc</span>
              </button>
            </div>
          </div>
        </div>
      </section>

      <!-- Правая колонка: Терминал вывода slmgr -->
      <section class="terminal-column">
        <div class="card terminal-card">
          <div class="terminal-header">
            <div class="term-title-group">
              <span class="term-dot dot-red"></span>
              <span class="term-dot dot-yellow"></span>
              <span class="term-dot dot-green"></span>
              <span class="term-title">Консоль вывода</span>
            </div>

            <div class="term-actions">
              <span v-if="isBusy" class="term-busy">
                <span class="spinner-small"></span> {{ currentAction }}...
              </span>
              <button class="term-btn" @click="copyLogs" :disabled="logs.length === 0">
                {{ copySuccess ? '✓' : '📋 Копировать' }}
              </button>
              <button class="term-btn" @click="clearLogs" :disabled="logs.length === 0">
                🗑️ Очистить
              </button>
            </div>
          </div>

          <div class="terminal-content-wrap">
            <canvas ref="matrixCanvasRef" class="matrix-canvas"></canvas>
            <div class="terminal-body" ref="terminalRef">
              <div v-if="logs.length === 0" class="term-empty">
                Ожидание команд...
              </div>
              <div
                v-for="(log, idx) in logs"
                :key="idx"
                class="term-line"
                :class="{
                  'line-exec': log.text.startsWith('▶') || log.text.startsWith('[EXEC]'),
                  'line-success': log.text.startsWith('✔') || log.text.includes('успешно') || log.text.includes('УСПЕШНО'),
                  'line-error': log.text.startsWith('✖') || log.text.startsWith('[ERROR]'),
                  'line-warn': log.text.startsWith('[WARNING]'),
                  'line-divider': log.text.startsWith('══')
                }"
              >
                <span class="term-time">{{ log.time }}</span>
                <span class="term-text">{{ log.text }}</span>
              </div>
            </div>
          </div>
        </div>
      </section>
    </main>

    <!-- Модальное окно характеристик ПК -->
    <div v-if="showSpecsModal" class="modal-backdrop" @click.self="closeSpecsModal">
      <div class="modal-dialog">
        <div class="modal-header">
          <div class="modal-title-group">
            <span class="modal-icon">💻</span>
            <h2 class="modal-title">Характеристики компьютера</h2>
          </div>
          <button class="modal-close" @click="closeSpecsModal" title="Закрыть">✕</button>
        </div>

        <div class="modal-body">
          <div v-if="isLoadingSpecs" class="specs-loading">
            <span class="spinner"></span>
            <span>Сбор данных о системе...</span>
          </div>

          <div v-else-if="specs" class="specs-grid">
            <!-- Процессор -->
            <div class="spec-card">
              <div class="spec-card-title">
                <span>⚡ Процессор (CPU)</span>
                <span class="spec-badge">{{ specs.cpuCores }} потоков</span>
              </div>
              <div class="spec-val-primary">{{ specs.cpuName }}</div>
            </div>

            <!-- Оперативная память -->
            <div class="spec-card">
              <div class="spec-card-title">
                <span>🧠 Оперативная память (RAM)</span>
                <span class="spec-badge">{{ specs.usedRam }} / {{ specs.totalRam }}</span>
              </div>
              <div class="spec-bar-wrapper">
                <div class="spec-bar-track">
                  <div class="spec-bar-fill" :style="{ width: specs.ramPercent + '%' }"></div>
                </div>
                <div class="spec-bar-labels">
                  <span>Занято: {{ specs.ramPercent }}%</span>
                  <span>Свободно: {{ specs.availRam }}</span>
                </div>
              </div>
            </div>

            <!-- Видеокарта -->
            <div class="spec-card">
              <div class="spec-card-title">
                <span>🎮 Видеокарта (GPU)</span>
              </div>
              <div class="spec-gpu-list">
                <div v-for="(gpu, idx) in specs.gpus" :key="idx" class="spec-gpu-item">
                  {{ gpu }}
                </div>
              </div>
            </div>

            <!-- Материнская плата и BIOS -->
            <div class="spec-card">
              <div class="spec-card-title">
                <span>🔌 Материнская плата</span>
              </div>
              <div class="spec-val-primary">{{ specs.motherboard }}</div>
              <div v-if="specs.biosInfo" class="spec-val-sub">BIOS: {{ specs.biosInfo }}</div>
            </div>

            <!-- Накопители -->
            <div class="spec-card spec-card-wide" v-if="specs.disks && specs.disks.length">
              <div class="spec-card-title">
                <span>💾 Накопители</span>
              </div>
              <div class="spec-disks-grid">
                <div v-for="disk in specs.disks" :key="disk.letter" class="spec-disk-item">
                  <div class="disk-meta">
                    <span class="disk-letter">{{ disk.letter }}</span>
                    <span class="disk-size">Свободно {{ disk.freeGb }} из {{ disk.totalGb }}</span>
                  </div>
                  <div class="spec-bar-track">
                    <div class="spec-bar-fill" :class="{ 'bar-warn': disk.percentUsed > 85 }" :style="{ width: disk.percentUsed + '%' }"></div>
                  </div>
                </div>
              </div>
            </div>

            <!-- Система и компьютер -->
            <div class="spec-card spec-card-wide">
              <div class="spec-card-title">
                <span>🪟 Операционная система</span>
              </div>
              <div class="spec-info-row">
                <div class="info-row-item"><span class="info-lbl">ОС:</span> {{ specs.osName }} {{ specs.osVersion }} ({{ specs.arch }})</div>
                <div class="info-row-item"><span class="info-lbl">Сборка:</span> {{ specs.osBuild }}</div>
                <div class="info-row-item"><span class="info-lbl">Имя ПК:</span> {{ specs.computerName }}</div>
                <div class="info-row-item"><span class="info-lbl">Пользователь:</span> {{ specs.userName }}</div>
                <div class="info-row-item"><span class="info-lbl">Время работы:</span> {{ specs.uptime }}</div>
              </div>
            </div>
          </div>
        </div>

        <div class="modal-footer">
          <div class="modal-footer-left">
            <button class="btn-modal-action" @click="copySpecsText">
              {{ copySpecsSuccess ? '✓ Скопировано' : '📋 Скопировать' }}
            </button>
            <button class="btn-modal-action" @click="handlePrintSpecsToConsole">
              ▶ Вывести в консоль
            </button>
            <button class="btn-modal-action" @click="refreshSpecs" :disabled="isLoadingSpecs">
              🔄 Обновить
            </button>
          </div>
          <button class="btn-modal-close" @click="closeSpecsModal">Закрыть</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style>
/* Базовые стили и переменные темы */
:root {
  --bg-app: #0d1117;
  --bg-card: rgba(22, 27, 34, 0.9);
  --bg-card-border: rgba(255, 255, 255, 0.08);
  --bg-input: #161b22;
  --accent-primary: #388bfd;
  --accent-gradient: linear-gradient(135deg, #1f6feb 0%, #238636 100%);
  --accent-gradient-hover: linear-gradient(135deg, #388bfd 0%, #2ea043 100%);
  --text-main: #f0f6fc;
  --text-muted: #8b949e;
  --border-color: #30363d;
  --color-green: #3fb950;
  --color-amber: #d29922;
  --color-red: #f85149;
  --font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  --font-mono: "Cascadia Code", "Consolas", monospace;
}

* {
  box-sizing: border-box;
  margin: 0;
  padding: 0;
}

body {
  font-family: var(--font-family);
  background-color: var(--bg-app);
  color: var(--text-main);
  height: 100vh;
  overflow: hidden;
  user-select: none;
}

.app-layout {
  display: flex;
  flex-direction: column;
  height: 100vh;
  padding: 10px 12px 12px;
  gap: 0;
  overflow: hidden;
}

.admin-status-box {
  display: flex;
  align-items: center;
  justify-content: center;
  margin-top: 2px;
  width: 100%;
}

.admin-status-box .badge {
  width: 100%;
  justify-content: center;
  padding: 6px 12px;
  font-size: 0.76rem;
  font-weight: 500;
}

.admin-status-box .badge-group {
  width: 100%;
  display: flex;
  gap: 6px;
}

.admin-status-box .badge-group .badge {
  flex: 1;
}

.badge-group {
  display: flex;
  align-items: center;
  gap: 6px;
}

.badge {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 3px 8px;
  border-radius: 16px;
  font-size: 0.74rem;
  font-weight: 500;
  border: 1px solid transparent;
}

.badge-admin {
  background: rgba(46, 160, 67, 0.15);
  color: #3fb950;
  border-color: rgba(46, 160, 67, 0.3);
}

.badge-warn {
  background: rgba(210, 153, 34, 0.15);
  color: #d29922;
  border-color: rgba(210, 153, 34, 0.3);
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}
.dot-green { background: #3fb950; box-shadow: 0 0 5px #3fb950; }
.dot-amber { background: #d29922; box-shadow: 0 0 5px #d29922; }

.btn-elevate {
  background: #21262d;
  color: #58a6ff;
  border: 1px solid #388bfd;
  padding: 3px 8px;
  border-radius: 5px;
  font-size: 0.72rem;
  cursor: pointer;
  transition: all 0.2s;
}
.btn-elevate:hover {
  background: #388bfd;
  color: white;
}

/* Layout Grid */
.main-content {
  display: grid;
  grid-template-columns: 370px minmax(0, 1fr);
  grid-template-rows: minmax(0, 1fr);
  gap: 12px;
  flex: 1;
  min-height: 0;
  max-height: 100%;
  overflow: hidden;
}

.controls-column {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: 0;
  overflow-y: auto;
}

.terminal-column {
  display: flex;
  flex-direction: column;
  height: 100%;
  max-height: 100%;
  min-height: 0;
  min-width: 0;
  overflow: hidden;
}

/* Cards */
.card {
  background: var(--bg-card);
  border: 1px solid var(--bg-card-border);
  border-radius: 9px;
  padding: 12px 14px;
}

.compact-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 5px;
}
.mt-8 {
  margin-top: 6px;
}

.form-label {
  font-size: 0.74rem;
  color: var(--text-muted);
}

.form-input {
  width: 100%;
  background: var(--bg-input);
  border: 1px solid var(--border-color);
  color: var(--text-main);
  padding: 7px 9px;
  border-radius: 6px;
  font-size: 0.82rem;
  outline: none;
  transition: border-color 0.2s;
}

.form-input:focus {
  border-color: var(--accent-primary);
}

.key-input-wrapper {
  display: flex;
  gap: 5px;
}

.key-input {
  font-family: var(--font-mono);
  letter-spacing: 0.04em;
  font-weight: 600;
  font-size: 0.82rem;
  color: #58a6ff;
}

.btn-icon {
  background: #21262d;
  border: 1px solid var(--border-color);
  color: var(--text-muted);
  width: 32px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.9rem;
  transition: all 0.2s;
  display: flex;
  align-items: center;
  justify-content: center;
}
.btn-icon:hover {
  color: var(--text-main);
  border-color: #58a6ff;
}

.label-with-action {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.btn-text-action {
  background: transparent;
  border: none;
  color: #58a6ff;
  font-size: 0.7rem;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 3px;
  padding: 1px 4px;
  border-radius: 4px;
  transition: all 0.2s;
}
.btn-text-action:hover:not(:disabled) {
  background: rgba(88, 166, 255, 0.12);
}
.btn-text-action:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.spin-anim {
  display: inline-block;
  animation: spin 1s linear infinite;
}

/* KMS Chips / Pills */
.chips-container {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  margin-bottom: 4px;
}

.chip {
  background: #21262d;
  border: 1px solid var(--border-color);
  color: var(--text-muted);
  padding: 3px 7px;
  border-radius: 5px;
  font-size: 0.72rem;
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
  display: inline-flex;
  align-items: center;
  gap: 5px;
}

.chip:hover {
  background: #30363d;
  color: var(--text-main);
  border-color: #8b949e;
}

.chip.active {
  background: rgba(56, 139, 253, 0.2);
  color: #58a6ff;
  border-color: #388bfd;
  font-weight: 500;
}

.chip-label {
  display: inline-block;
}

.ping-badge {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 0.65rem;
  font-family: var(--font-mono);
  padding: 0 4px;
  border-radius: 3px;
  background: rgba(0, 0, 0, 0.35);
}

.ping-online {
  color: #3fb950;
}

.ping-offline {
  color: #f85149;
}

.ping-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  display: inline-block;
}
.ping-dot.dot-green {
  background: #3fb950;
  box-shadow: 0 0 4px #3fb950;
}
.ping-dot.dot-red {
  background: #f85149;
  box-shadow: 0 0 4px #f85149;
}
.ping-loading {
  background: #58a6ff;
  animation: pulse 1s infinite;
}

@keyframes pulse {
  0% { opacity: 0.3; }
  50% { opacity: 1; }
  100% { opacity: 0.3; }
}

.server-input-wrapper {
  display: flex;
  gap: 5px;
}

.server-input {
  flex: 1;
  font-size: 0.8rem;
  color: #e6edf3;
}

.spinner-tiny {
  width: 11px;
  height: 11px;
  border: 2px solid rgba(88, 166, 255, 0.3);
  border-top-color: #58a6ff;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  display: inline-block;
}

/* Troubleshooting / Сброс */
.troubleshoot-box {
  margin-top: 4px;
  display: flex;
  flex-direction: column;
  gap: 5px;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  padding-top: 6px;
}

.btn-troubleshoot-toggle {
  background: #161b22;
  border: 1px solid var(--border-color);
  color: var(--text-muted);
  font-size: 0.72rem;
  padding: 5px 8px;
  border-radius: 5px;
  cursor: pointer;
  display: flex;
  justify-content: space-between;
  align-items: center;
  transition: all 0.2s;
  width: 100%;
}
.btn-troubleshoot-toggle:hover {
  color: var(--text-main);
  border-color: #58a6ff;
}
.btn-troubleshoot-toggle.active {
  color: #58a6ff;
  border-color: #388bfd;
  background: rgba(56, 139, 253, 0.08);
}

.troubleshoot-toggle-title {
  font-weight: 500;
}

.toggle-arrow {
  font-size: 0.65rem;
  opacity: 0.7;
}

.troubleshoot-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 5px;
}

.btn-trouble {
  background: #1f242c;
  border: 1px solid var(--border-color);
  color: #c9d1d9;
  padding: 6px 4px;
  border-radius: 5px;
  font-size: 0.71rem;
  cursor: pointer;
  transition: all 0.2s;
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
  font-weight: 500;
}
.btn-trouble:hover:not(:disabled) {
  background: #30363d;
  border-color: #8b949e;
  color: white;
}
.btn-trouble.btn-danger:hover:not(:disabled) {
  background: rgba(248, 81, 73, 0.15);
  border-color: #f85149;
  color: #ff7b72;
}
.btn-trouble:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

/* Action Buttons */
.action-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.btn-primary {
  width: 100%;
  background: var(--accent-gradient);
  color: white;
  border: none;
  border-radius: 7px;
  padding: 10px 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  cursor: pointer;
  box-shadow: 0 3px 12px rgba(35, 134, 54, 0.3);
  transition: all 0.2s;
}
.btn-primary:hover:not(:disabled) {
  background: var(--accent-gradient-hover);
  transform: translateY(-1px);
  box-shadow: 0 5px 16px rgba(35, 134, 54, 0.4);
}
.btn-primary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-icon-large {
  font-size: 1.3rem;
}

.btn-text {
  display: flex;
  flex-direction: column;
  text-align: left;
}
.btn-text strong {
  font-size: 0.92rem;
}
.btn-text small {
  font-size: 0.7rem;
  opacity: 0.85;
}

.btn-grid-two {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}

.btn-secondary {
  background: #21262d;
  border: 1px solid var(--border-color);
  color: #c9d1d9;
  padding: 8px 10px;
  border-radius: 6px;
  font-size: 0.78rem;
  font-weight: 500;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}
.btn-secondary:hover:not(:disabled) {
  background: #30363d;
  color: white;
  border-color: #8b949e;
}
.btn-secondary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* Terminal Component */
.terminal-card {
  display: flex;
  flex-direction: column;
  height: 100%;
  max-height: 100%;
  min-height: 0;
  min-width: 0;
  padding: 0;
  overflow: hidden;
  background: #090d13;
  position: relative;
}

.terminal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 7px 12px;
  background: #11161f;
  border-bottom: 1px solid var(--border-color);
  flex-shrink: 0;
  position: relative;
  z-index: 2;
}

.term-title-group {
  display: flex;
  align-items: center;
  gap: 5px;
}

.term-dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
}
.term-dot.dot-red { background: #ff5f56; }
.term-dot.dot-yellow { background: #ffbd2e; }
.term-dot.dot-green { background: #27c93f; }

.term-title {
  margin-left: 5px;
  font-size: 0.76rem;
  color: var(--text-muted);
  font-weight: 500;
}

.term-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

.term-busy {
  font-size: 0.72rem;
  color: #58a6ff;
  display: flex;
  align-items: center;
  gap: 5px;
}

.term-btn {
  background: #21262d;
  border: 1px solid var(--border-color);
  color: var(--text-muted);
  font-size: 0.7rem;
  padding: 2px 7px;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s;
}
.term-btn:hover:not(:disabled) {
  color: var(--text-main);
  border-color: #58a6ff;
}
.term-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.terminal-content-wrap {
  position: relative;
  flex: 1 1 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: #090d13;
}

.matrix-canvas {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  opacity: 0.16;
  z-index: 0;
}

.terminal-body {
  position: relative;
  z-index: 1;
  flex: 1 1 0;
  min-height: 0;
  padding: 10px 12px;
  font-family: var(--font-mono);
  font-size: 0.78rem;
  line-height: 1.4;
  overflow-y: auto;
  overflow-x: hidden;
  user-select: text;
  white-space: pre-wrap;
  word-break: break-word;
  background: transparent;
}

.term-empty {
  color: #484f58;
  font-style: italic;
  padding-top: 15px;
  text-align: center;
}

.term-line {
  display: flex;
  gap: 8px;
  margin-bottom: 2px;
  color: #c9d1d9;
}

.term-time {
  color: #484f58;
  font-size: 0.7rem;
  user-select: none;
  flex-shrink: 0;
}

.term-text {
  flex: 1;
}

.line-exec {
  color: #58a6ff;
  font-weight: 600;
}

.line-success {
  color: #3fb950;
  font-weight: 600;
}

.line-error {
  color: #f85149;
  font-weight: 600;
}

.line-warn {
  color: #d29922;
}

.line-divider {
  color: #30363d;
  user-select: none;
}

/* Spinners */
.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: white;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.spinner-small {
  width: 10px;
  height: 10px;
  border: 2px solid rgba(88, 166, 255, 0.3);
  border-top-color: #58a6ff;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  display: inline-block;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

::-webkit-scrollbar {
  width: 5px;
  height: 5px;
}
::-webkit-scrollbar-track {
  background: transparent;
}
::-webkit-scrollbar-thumb {
  background: #30363d;
  border-radius: 3px;
}
::-webkit-scrollbar-thumb:hover {
  background: #484f58;
}

/* Кнопка Характеристики в карточке действий */
.btn-specs-full {
  width: 100%;
  padding: 8px 10px;
  font-size: 0.78rem;
  font-weight: 500;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  background: #21262d;
  border: 1px solid var(--border-color);
  color: #c9d1d9;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
}
.btn-specs-full:hover {
  background: #30363d;
  border-color: #58a6ff;
  color: #58a6ff;
}

/* Модальное окно характеристик */
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.75);
  backdrop-filter: blur(5px);
  z-index: 9999;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  animation: fadeIn 0.15s ease-out;
}

.modal-dialog {
  width: 680px;
  max-width: 96vw;
  max-height: 88vh;
  background: #161b22;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 12px;
  display: flex;
  flex-direction: column;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.6);
  overflow: hidden;
  animation: slideUp 0.2s ease-out;
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 18px;
  background: #0d1117;
  border-bottom: 1px solid var(--border-color);
}

.modal-title-group {
  display: flex;
  align-items: center;
  gap: 8px;
}
.modal-icon {
  font-size: 1.1rem;
}
.modal-title {
  font-size: 0.96rem;
  font-weight: 600;
  color: #f0f6fc;
}
.modal-close {
  background: transparent;
  border: none;
  color: var(--text-muted);
  font-size: 1.1rem;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 4px;
  line-height: 1;
  transition: all 0.15s;
}
.modal-close:hover {
  color: #f0f6fc;
  background: #21262d;
}

.modal-body {
  padding: 16px 18px;
  overflow-y: auto;
  flex: 1;
}

.specs-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 40px;
  color: var(--text-muted);
  font-size: 0.86rem;
}

.specs-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.spec-card {
  background: #0d1117;
  border: 1px solid var(--border-color);
  border-radius: 8px;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.spec-card-wide {
  grid-column: 1 / -1;
}

.spec-card-title {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.76rem;
  color: var(--text-muted);
  font-weight: 500;
}

.spec-badge {
  background: rgba(56, 139, 253, 0.15);
  color: #58a6ff;
  font-size: 0.7rem;
  padding: 2px 6px;
  border-radius: 4px;
}

.spec-val-primary {
  font-size: 0.88rem;
  font-weight: 600;
  color: #f0f6fc;
  word-break: break-word;
}

.spec-val-sub {
  font-size: 0.76rem;
  color: var(--text-muted);
}

.spec-bar-wrapper {
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.spec-bar-track {
  width: 100%;
  height: 6px;
  background: #21262d;
  border-radius: 3px;
  overflow: hidden;
}

.spec-bar-fill {
  height: 100%;
  background: var(--accent-gradient);
  border-radius: 3px;
  transition: width 0.3s ease;
}
.spec-bar-fill.bar-warn {
  background: linear-gradient(90deg, #d29922, #f85149);
}

.spec-bar-labels {
  display: flex;
  justify-content: space-between;
  font-size: 0.72rem;
  color: var(--text-muted);
}

.spec-gpu-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.spec-gpu-item {
  font-size: 0.84rem;
  font-weight: 600;
  color: #f0f6fc;
}

.spec-disks-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}

.spec-disk-item {
  background: #161b22;
  border: 1px solid var(--border-color);
  padding: 8px 10px;
  border-radius: 6px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.disk-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.disk-letter {
  font-family: var(--font-mono);
  font-weight: 700;
  color: #58a6ff;
  font-size: 0.85rem;
}

.disk-size {
  font-size: 0.72rem;
  color: var(--text-muted);
}

.spec-info-row {
  display: flex;
  flex-wrap: wrap;
  gap: 12px 18px;
  font-size: 0.78rem;
  color: #e6edf3;
}

.info-row-item {
  display: flex;
  gap: 4px;
}

.info-lbl {
  color: var(--text-muted);
}

.modal-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 18px;
  background: #0d1117;
  border-top: 1px solid var(--border-color);
}

.modal-footer-left {
  display: flex;
  gap: 8px;
}

.btn-modal-action {
  background: #21262d;
  border: 1px solid var(--border-color);
  color: #c9d1d9;
  font-size: 0.74rem;
  padding: 5px 10px;
  border-radius: 5px;
  cursor: pointer;
  transition: all 0.2s;
}
.btn-modal-action:hover:not(:disabled) {
  background: #30363d;
  color: white;
  border-color: #58a6ff;
}

.btn-modal-close {
  background: #238636;
  border: none;
  color: white;
  font-size: 0.76rem;
  font-weight: 600;
  padding: 6px 14px;
  border-radius: 5px;
  cursor: pointer;
  transition: all 0.2s;
}
.btn-modal-close:hover {
  background: #2ea043;
}

@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

@keyframes slideUp {
  from { opacity: 0; transform: translateY(12px) scale(0.98); }
  to { opacity: 1; transform: translateY(0) scale(1); }
}
</style>
