<script setup lang="ts">
import { ref } from 'vue'
import { t } from '../i18n'
import APIClient from '../services/APIClient'
import { WKSDK } from 'wukongimjssdk';
import { establishSession, loadSession } from '../services/session'


const getUrlParam = (name: string) => {
  var reg = new RegExp("(^|&)" + name + "=([^&]*)(&|$)"); // 构造一个含有目标参数的正则表达式对象
  var r = window.location.search.substr(1).match(reg); // 匹配目标参数
  if (r != null) return unescape(r[2]);
  return null; // 返回参数值
}

var apiurl = getUrlParam("apiurl")


if (!apiurl || apiurl?.trim() == "") {
  apiurl = import.meta.env.DEV ? "http://127.0.0.1:5001" : window.location.origin
} else {
  // 去掉 apiurl后的 “/”
  if (apiurl && apiurl.endsWith("/")) {
    apiurl = apiurl.substring(0, apiurl.length - 1)
  }
}


const saved = loadSession(window.sessionStorage)
const apiAddr = ref(apiurl || saved?.apiURL || '')
const username = ref(saved?.uid || '')
const password = ref(saved?.token || '')
const createDemoCredentials = ref(false)
const submitting = ref(false)
const errorMessage = ref('')

const login = async () => {
  if (submitting.value) return
  submitting.value = true
  errorMessage.value = ''
  try {
    const session = await establishSession(window.sessionStorage, {
      apiURL: apiAddr.value, uid: username.value, token: password.value,
    }, createDemoCredentials.value, async session => {
      APIClient.shared.config.apiURL = session.apiURL
      await APIClient.shared.post('/user/token', {
        uid: session.uid, token: session.token, device_flag: 1, device_level: 0,
      })
    })
    APIClient.shared.config.apiURL = session.apiURL
    // Recreate the SDK singleton so another account cannot inherit cached messages.
    window.location.replace(window.location.pathname + window.location.search + '#/chat')
  } catch {
    errorMessage.value = t('loginFailed')
  } finally {
    submitting.value = false
  }
}



</script>
<template>
  <div class="hello">
    <div>
      <a href="https://githubim.com" target="_blank">
        <img src="/logo.png" class="logo" :alt="t('logo')" />
      </a>
    </div>
    <p>
      {{ t('sdkVersion', { version: WKSDK.shared().config.sdkVersion }) }}
    </p>
    <div class="form">
      <div class="item">
        <div class="label">
          <label>{{ t('apiAddress') }}</label>
        </div>
        <div class="field">
          <input type="text" :placeholder="t('apiAddressPlaceholder')" v-model="apiAddr" />
        </div>
      </div>
      <div class="item">
        <div class="label">
          <label>{{ t('username') }}</label>
        </div>
        <div class="field">
          <input type="text" :placeholder="t('usernamePlaceholder')" v-model="username" />
        </div>
      </div>
      <div class="item">
        <div class="label">
          <label>{{ t('password') }}</label>
        </div>
        <div class="field">
          <input type="password" autocomplete="off" :placeholder="t('passwordPlaceholder')" v-model="password" />
        </div>
      </div>
      <p>{{ t('existingTokenHint') }}</p>
      <label><input type="checkbox" v-model="createDemoCredentials" /> {{ t('createDemoCredentials') }}</label>
      <p v-if="errorMessage" role="alert">{{ errorMessage }}</p>
      <button class="submit" :disabled="submitting" v-on:click="login">{{ t('login') }}</button>
    </div>
  </div>
</template>

<style scoped>
.form {
  width: 100%;
  margin-top: 40px;
}

.item {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  margin-top: 20px;
}

.item label {
  font-size: 17px;
}

.field input {
  width: min(300px, 55vw);
  height: 30px;
  border: none;
  margin-left: 20px;
  font-size: 17px;
}

.form .submit {
  margin-top: 40px;
  height: 60px;
  min-width: 300px;
  max-width: 600px;
  width: 80%;
  border: none;
  border-radius: 4px;
  color: white;
  background-color: rgb(228, 98, 64);
  font-size: 20px;
  cursor: pointer;
}

.logo {
  height: 6em;
  padding: 1.5em;
  will-change: filter;
  transition: filter 300ms;
}

.logo:hover {
  filter: drop-shadow(0 0 2em #646cffaa);
}

.logo.vue:hover {
  filter: drop-shadow(0 0 2em #42b883aa);
}
</style>
