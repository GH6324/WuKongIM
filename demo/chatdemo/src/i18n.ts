export type Locale = 'zh' | 'en'

type BrowserLanguages = { languages?: readonly string[], language?: string }

// A supported URL language overrides browser preferences; English is the fallback.
export function detectLocale(browser?: BrowserLanguages, languageOverride?: string | null): Locale {
  if (languageOverride === 'en' || languageOverride === 'zh') return languageOverride
  const languages = [...(browser?.languages ?? []), browser?.language ?? '']
  for (const language of languages) {
    const base = language.trim().toLowerCase().split(/[-_]/)[0]
    if (base === 'zh' || base === 'en') return base
  }
  return 'en'
}

export const locale = detectLocale(
  typeof navigator === 'undefined' ? undefined : navigator,
  typeof window === 'undefined' ? undefined : new URLSearchParams(window.location.search).get('lang'),
)

const en = {
  appTitle: 'WuKongIM Demo',
  sdkVersion: 'WuKongIM Demo, SDK version: [v{version}]',
  logo: 'WuKongIM logo',
  apiAddress: 'API base URL',
  apiAddressPlaceholder: 'Enter the API base URL',
  username: 'Account',
  usernamePlaceholder: 'Any unique account for this demo',
  password: 'Password',
  passwordPlaceholder: 'Any password for this demo',
  login: 'Log in',
  logout: 'Log out',
  chatList: 'Chats',
  notConnected: '{uid} (Not connected)',
  connected: '{uid} (Connected)',
  connectedNode: '{uid} (Connected - Node: {node})',
  disconnected: '{uid} (Disconnected)',
  starProject: 'Give us a star on GitHub',
  chooseChat: 'Start a chat',
  groupChatTitle: 'Group: {id}',
  directChatTitle: 'Direct chat: {id}',
  directChat: 'Direct chat',
  groupChat: 'Group chat',
  recipientPlaceholder: 'Enter the recipient account',
  groupPlaceholder: 'Enter the group ID',
  messagePlaceholder: 'Enter a message',
  sending: 'Sending',
  customMessage: 'Custom message',
  send: 'Send',
  confirm: 'OK',
  orderNumber: 'Order: {number}',
  orderQuantity: 'Quantity: {count}',
  sampleProduct: 'Cocoa lemon milk tea',
  orderDigest: '[Order message]',
  cardDigest: '[Card message]',
  streamDigest: '[Stream message]',
  messageDigest: '[Message]',
  unknownMessageDigest: '[Unknown message]',
  imageDigest: '[Image]',
  requestNotFound: 'Request URL not found (404)',
  unknownError: 'Unknown error',
  justNow: 'Just now',
  yesterday: 'Yesterday',
  dayBeforeYesterday: '2 days ago',
}

type MessageKey = keyof typeof en

const zh: Record<MessageKey, string> = {
  appTitle: '悟空IM演示程序',
  sdkVersion: '悟空IM演示程序，当前SDK版本：[v{version}]',
  logo: '悟空IM标志',
  apiAddress: 'API基地址',
  apiAddressPlaceholder: '请输入API基地址',
  username: '登录账号',
  usernamePlaceholder: '演示下，随便输，唯一即可',
  password: '登录密码',
  passwordPlaceholder: '演示下，随便输',
  login: '登录',
  logout: '退出',
  chatList: '聊天列表',
  notConnected: '{uid}(未连接)',
  connected: '{uid}(连接成功)',
  connectedNode: '{uid}(连接成功-节点:{node})',
  disconnected: '{uid}(断开)',
  starProject: '吴彦祖，点个Star呗',
  chooseChat: '与谁会话？',
  groupChatTitle: '群{id}',
  directChatTitle: '单聊{id}',
  directChat: '单聊',
  groupChat: '群聊',
  recipientPlaceholder: '请输入对方登录名',
  groupPlaceholder: '请输入群组ID',
  messagePlaceholder: '请输入消息',
  sending: '发送中',
  customMessage: '自定义消息',
  send: '发送',
  confirm: '确定',
  orderNumber: '订单号：{number}',
  orderQuantity: '共{count}件',
  sampleProduct: '可可柠檬鲜美奶茶',
  orderDigest: '[订单消息]',
  cardDigest: '[卡片消息]',
  streamDigest: '[流消息]',
  messageDigest: '[消息]',
  unknownMessageDigest: '[未知消息]',
  imageDigest: '[图片]',
  requestNotFound: '请求地址没有找到（404）',
  unknownError: '未知错误',
  justNow: '刚刚',
  yesterday: '昨天',
  dayBeforeYesterday: '前天',
}

export const messages: Record<Locale, Record<MessageKey, string>> = { en, zh }

// Interpolate only catalog placeholders; user values are never translated or evaluated.
export function translate(language: Locale, key: MessageKey, params: Record<string, string | number> = {}): string {
  return messages[language][key].replace(/\{(\w+)\}/g, (placeholder, name: string) =>
    Object.prototype.hasOwnProperty.call(params, name) ? String(params[name]) : placeholder)
}

export function t(key: MessageKey, params?: Record<string, string | number>): string {
  return translate(locale, key, params)
}

const dateFormatters = Object.fromEntries((['zh', 'en'] as const).map(language => [language, {
  time: new Intl.DateTimeFormat(language, { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }),
  weekday: new Intl.DateTimeFormat(language, { weekday: 'long' }),
  date: new Intl.DateTimeFormat(language, { year: 'numeric', month: 'numeric', day: 'numeric' }),
}])) as Record<Locale, Record<'time' | 'weekday' | 'date', Intl.DateTimeFormat>>

// Compare calendar days in the user's time zone, including month/year and DST boundaries.
export function formatConversationTime(timestamp: number, language: Locale = locale, now = new Date()): string {
  const date = new Date(timestamp)
  const sameDay = (other: Date) => date.getFullYear() === other.getFullYear()
    && date.getMonth() === other.getMonth() && date.getDate() === other.getDate()
  const format = dateFormatters[language]
  const time = format.time.format(date)
  const elapsed = now.getTime() - timestamp
  if (sameDay(now)) {
    return elapsed >= 0 && elapsed < 60_000 ? translate(language, 'justNow') : time
  }
  for (const days of [1, 2]) {
    const previous = new Date(now)
    previous.setDate(previous.getDate() - days)
    if (sameDay(previous)) {
      return `${translate(language, days === 1 ? 'yesterday' : 'dayBeforeYesterday')} ${time}`
    }
  }
  const day = elapsed > 0 && elapsed <= 7 * 24 * 60 * 60 * 1000
    ? format.weekday.format(date) : format.date.format(date)
  return `${day} ${time}`
}
