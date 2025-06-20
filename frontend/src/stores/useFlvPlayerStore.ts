// stores/useFlvPlayerStore.ts

import { defineStore } from 'pinia'
import { ref } from 'vue'
import flvjs from 'flv.js'

export const useFlvPlayerStore = defineStore('flvPlayer', () => {
    // 当前拉流地址
    const flvUrl = ref<string>('')

    // 是否正在播放
    const isPlaying = ref<boolean>(false)

    // 播放器实例
    const playerInstance = ref<any>(null)

    // 设置拉流地址
    function setFlvUrl(url: string) {
        flvUrl.value = url
    }

    // 设置播放状态
    function setIsPlaying(status: boolean) {
        isPlaying.value = status
    }

    // 设置播放器实例
    function setPlayer(player: any) {
        playerInstance.value = player
    }

    // 获取当前播放器
    function getPlayer() {
        return playerInstance.value
    }

    // 销毁播放器
    function destroyPlayer() {
        if (playerInstance.value) {
            try {
                playerInstance.value.pause()
                playerInstance.value.unload()
                playerInstance.value.detachMediaElement()
                playerInstance.value = null
            } catch (e) {
                console.error('销毁播放器失败:', e)
            }
        }
    }

    // 恢复播放器（用于切换路由后）
    function restorePlayer(videoEl: HTMLVideoElement) {
        if (playerInstance.value && !playerInstance.value.isDestroyed()) {
            playerInstance.value.attachMediaElement(videoEl)
            playerInstance.value.load()
            playerInstance.value.play()
        }
    }

    return {
        flvUrl,
        isPlaying,
        playerInstance,
        setFlvUrl,
        setIsPlaying,
        setPlayer,
        getPlayer,
        destroyPlayer,
        restorePlayer,
    }
})