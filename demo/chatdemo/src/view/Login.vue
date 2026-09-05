<script setup lang="ts">
import { ref } from 'vue'
import { t } from '../i18n'
import APIClient from '../services/APIClient'
import { useRouter, useRoute } from "vue-router";
import { WKSDK } from 'wukongimjssdk';
const router = useRouter();


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


console.log("apiurl--->", apiurl)

// defineProps<{ msg: string }>()

const count = ref(0)
const apiAddr = ref(apiurl || '')
const username = ref('')
const password = ref('')

const login = () => {
  APIClient.shared.config.apiURL = apiAddr.value
  // 注意：这里的登录接口是悟空IM的演示接口，仅供演示使用，这些接口不应该暴露给前端，应该由后端封装后提供给前端
  APIClient.shared.post('/user/token', {
    uid: username.value, // 第三方服务端的用户唯一uid
    token: password.value || "default111111", // 第三方服务端的用户的token
    device_flag: 1, // 设备标识  0.app 1.web （相同用户相同设备标记的主设备登录会互相踢，从设备将共存）
    device_level: 0,  // 设备等级 0.为从设备 1.为主设备
  }).then((res) => {
    console.log(res)
    router.push({ path: '/chat', query: { uid: username.value, token: password.value } })
  }).catch((err) => {
    alert(err.msg)
  })
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
          <input type="text" :placeholder="t('passwordPlaceholder')" v-model="password" />
        </div>
      </div>
      <button class="submit" v-on:click="login">{{ t('login') }}</button>
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
