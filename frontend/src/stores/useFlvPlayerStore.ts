// stores/useFlvPlayerStore.ts

import { defineStore } from 'pinia'
import { ref } from 'vue'
import flvjs from 'flv.js'

export const useFlvPlayerStore = defineStore('flvPlayer', () => {
    // 当前拉流地址
    const flvUrl = ref<string>('')

    // 设置拉流地址
    function setFlvUrl(url: string) {
        flvUrl.value = url
    }

    return {
        flvUrl,
        setFlvUrl,
    }
})