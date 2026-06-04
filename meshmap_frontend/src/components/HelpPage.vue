<template>
  <section class="help-page">
    <div class="panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Help</p>
          <h2>如何连接 MQTT</h2>
        </div>
      </div>

      <div class="help-content">
        <section>
          <h3>连接地址</h3>
          <p>将 Meshtastic 设备连接到本服务提供的 MQTT broker。</p>
          <ul>
            <li>默认地址：<strong>mesh.gat-iot.com</strong></li>
            <li>默认端口：<strong>1883</strong></li>
            <li>用户名称：<strong>meshdev</strong></li>
            <li>密码：<strong>large4cats</strong></li>
          </ul>
        </section>

        <section>
          <h3>频道加密要求</h3>
          <p>为了让服务能够解析 Meshtastic MQTT payload，频道需要满足以下任一条件：</p>
          <ul>
            <li>频道不加密。</li>
            <li>使用 Meshtastic 默认 PSK：<strong>AQ==</strong>。</li>
          </ul>
          <p>如果使用自定义加密密钥，数据可能会被判定为无法解密并丢弃。</p>
        </section>

        <section>
          <h3>反馈问题</h3>
          <p>如果遇到 bug，请在 GitHub <a href="https://github.com/wuwenfengmi1998/meshtastic_mqtt_server" target="_blank" rel="noopener noreferrer">提交 issue</a>，或联系邮箱 <a href="mailto:kevin@lmve.net">kevin@lmve.net</a>。</p>
        </section>

      </div>
    </div>
  </section>
</template>
