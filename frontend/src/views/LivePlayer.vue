<template>
  <div class="video-container">
    <div class="controls">
      <input v-model="videoUrl" type="text" placeholder="Enter video URL (e.g., http://127.0.0.1:8080/live/test110.flv)" />
      <button @click="initPlayer">Play</button>
    </div>
    <div id="mse" />
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import Player from "xgplayer";
import "xgplayer/dist/index.min.css";
import FlvPlugin from 'xgplayer-flv'

defineOptions({
  name: "VideoPage"
});

const videoUrl = ref("http://127.0.0.1:8080/live/test110.flv");
let player: Player | null = null;

const initPlayer = () => {
  // Destroy previous player if exists
  if (player) {
    player.destroy();
    player = null;
  }

  if (videoUrl.value) {
    player = new Player({
      id: "mse",
      lang: "zh",
      volume: 0,
      autoplay: true,
      screenShot: true,
      plugins: [FlvPlugin],
      url: videoUrl.value,
      fluid: true,
      playbackRate: [0.5, 0.75, 1, 1.5, 2]
    });
  }
};
</script>

<style scoped>
.video-container {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.controls {
  display: flex;
  gap: 10px;
  margin-bottom: 10px;
}

.controls input {
  flex: 1;
  padding: 8px;
  border: 1px solid #ccc;
  border-radius: 4px;
}

.controls button {
  padding: 8px 16px;
  background-color: #1890ff;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
}

.controls button:hover {
  background-color: #40a9ff;
}

#mse {
  flex: auto;
  min-height: 0;
}
</style>