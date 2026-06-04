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
          <p>将 Meshtastic 设备或客户端连接到本服务提供的 MQTT broker。</p>
          <ul>
            <li>默认地址：<strong>localhost</strong></li>
            <li>默认端口：<strong>1883</strong></li>
            <li>如果部署在其他机器，请使用服务器 IP 或域名。</li>
          </ul>
        </section>

        <section>
          <h3>频道加密要求</h3>
          <p>为了让服务能够解析 Meshtastic MQTT payload，频道需要满足以下任一条件：</p>
          <ul>
            <li>频道不加密。</li>
            <li>使用 Meshtastic 默认 PSK：<strong>AQ==</strong>。</li>
          </ul>
          <p>如果使用自定义加密密钥，需要在服务配置中设置对应的 <strong>meshtastic.psk</strong>，否则数据可能会被判定为无法解密并丢弃。</p>
        </section>

        <section>
          <h3>Topic 说明</h3>
          <p>客户端发布到 Meshtastic MQTT topic 后，服务会先解析 payload。解析成功的数据会被转发并写入数据库；解析失败的数据会被丢弃并记录到后台的丢弃数据页面。</p>
        </section>
      </div>
    </div>
  </section>
</template>
