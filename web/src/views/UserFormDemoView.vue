<template>
  <div class="demo-page">
    <!-- ========== 左侧卡片：传统后台表单 ========== -->
    <section class="card card-trad">
      <div class="card-titlebar">
        <span class="dots">
          <span class="dot red" aria-hidden="true" />
          <span class="dot yellow" aria-hidden="true" />
          <span class="dot green" aria-hidden="true" />
        </span>
        <span class="card-title">传统后台表单</span>
      </div>

      <form @submit.prevent>
        <!-- 1 账号 -->
        <div class="trad-field">
          <label class="trad-label" for="tf1"><span class="req">*</span> 账号</label>
          <input id="tf1" class="trad-input" type="text" readonly placeholder="请填写账号">
        </div>

        <!-- 2 真实姓名 -->
        <div class="trad-field">
          <label class="trad-label" for="tf2"><span class="req">*</span> 真实姓名</label>
          <input id="tf2" class="trad-input" type="text" readonly placeholder="请填写真实姓名">
        </div>

        <!-- 3 手机号码 -->
        <div class="trad-field">
          <label class="trad-label" for="tf3"><span class="req">*</span> 手机号码</label>
          <input id="tf3" class="trad-input" type="text" readonly placeholder="请填写手机号">
        </div>

        <!-- 4 所属区域 -->
        <div class="trad-field">
          <label class="trad-label"><span class="req">*</span> 所属区域</label>
          <div class="trad-row">
            <select class="trad-select" aria-label="选择省" disabled>
              <option selected disabled>选择省</option>
            </select>
            <select class="trad-select" aria-label="选择市" disabled>
              <option selected disabled>选择市</option>
            </select>
          </div>
        </div>

        <!-- 5 角色 -->
        <div class="trad-field">
          <label class="trad-label" for="tf5"><span class="req">*</span> 角色</label>
          <select id="tf5" class="trad-select" disabled>
            <option selected disabled>选择角色</option>
          </select>
        </div>

        <!-- 6 客户端版本 -->
        <div class="trad-field">
          <label class="trad-label" for="tf6"><span class="req">*</span> 客户端版本</label>
          <select id="tf6" class="trad-select sel" disabled>
            <option selected>通用</option>
            <option>专业版</option>
            <option>精简版</option>
          </select>
        </div>

        <!-- 7 邮箱地址 -->
        <div class="trad-field">
          <label class="trad-label" for="tf7">邮箱地址</label>
          <input id="tf7" class="trad-input" type="text" readonly placeholder="请填写邮箱地址">
        </div>

        <!-- 8 账号密码 -->
        <div class="trad-field">
          <label class="trad-label" for="tf8">账号密码</label>
          <input id="tf8" class="trad-input" type="text" readonly placeholder="请设置账号密码，非必填">
        </div>

        <!-- 9 确认密码 -->
        <div class="trad-field">
          <label class="trad-label" for="tf9">确认密码</label>
          <input id="tf9" class="trad-input" type="text" readonly placeholder="请确认账号密码，非必填">
        </div>

        <!-- 10 账号有效期 -->
        <div class="trad-field">
          <label class="trad-label" for="tf10"><span class="req">*</span> 账号有效期</label>
          <div class="date-wrap">
            <span class="date-icon" aria-hidden="true">📅</span>
            <input id="tf10" class="trad-input date-val" type="text" readonly value="2099-12-31">
          </div>
        </div>

        <button class="trad-submit" type="button" disabled>提交</button>
      </form>
    </section>

    <!-- ========== 右侧卡片：iOS + Notion 风格 ========== -->
    <section class="card card-modern">
      <div class="card-titlebar">
        <span class="dots">
          <span class="dot red" aria-hidden="true" />
          <span class="dot yellow" aria-hidden="true" />
          <span class="dot green" aria-hidden="true" />
        </span>
        <span class="card-title">iOS + Notion 风格</span>
      </div>

      <div class="modern-hero">
        <h1 class="modern-title">新建用户</h1>
        <p class="modern-sub">创建后，用户将收到短信邀请。</p>
      </div>

      <form @submit.prevent>
        <!-- 姓名 -->
        <div class="modern-field">
          <label class="modern-label" for="mf1">姓名</label>
          <input id="mf1" class="modern-input" type="text" readonly placeholder="例如：张三">
        </div>

        <!-- 手机号 -->
        <div class="modern-field mt-phone">
          <label class="modern-label" for="mf2">手机号</label>
          <input id="mf2" class="modern-input" type="text" readonly placeholder="138 0000 0000">
          <p class="helper">将用作登录账号，首次登录通过验证码验证。</p>
        </div>

        <!-- 角色 -->
        <div class="modern-field mt-role">
          <label class="modern-label role-label">角色</label>
          <div class="pills">
            <span class="pill active">普通用户</span>
            <span class="pill idle">管理员</span>
            <span class="pill idle">审计员</span>
          </div>
        </div>

        <!-- 高级设置 -->
        <div class="advanced">
          <div class="advanced-head">
            <span class="advanced-text">高级设置</span>
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true" class="chevron">
              <path d="M4 6L8 10L12 6" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
          </div>
          <p class="advanced-desc">邮箱、有效期、区域等</p>
        </div>

        <button class="modern-primary" type="button" disabled>创建用户</button>
        <button class="modern-cancel" type="button" disabled>取消</button>
      </form>
    </section>
  </div>
</template>

<style scoped>
/* ===== box-sizing 安全 ===== */
*,
*::before,
*::after {
  box-sizing: border-box;
}

/* ===== 页面容器 ===== */
.demo-page {
  min-height: 100vh;
  background: #F2F2F7;
  padding: 40px 20px;
  display: flex;
  justify-content: center;
  align-items: flex-start;
  gap: 40px;
  flex-wrap: wrap;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

/* ===== 卡片通用 ===== */
.card {
  width: 380px;
  background: #FFFFFF;
}

/* ===== 卡片标题栏 ===== */
.card-titlebar {
  display: flex;
  align-items: center;
  margin-bottom: 24px;
}
.dots {
  display: flex;
  gap: 8px;
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}
.dot.red { background: #FF3B30; }
.dot.yellow { background: #FFCC00; }
.dot.green { background: #34C759; }
.card-title {
  font-size: 14px;
  font-weight: 600;
  color: #1C1C1E;
  margin-left: 8px;
}

/* ===== 左侧卡片：传统后台表单 ===== */
.card-trad {
  border-radius: 12px;
  padding: 28px;
  box-shadow: 0 1px 3px rgba(0,0,0,0.08);
}
.trad-field {
  margin-bottom: 16px;
}
.trad-label {
  display: block;
  font-size: 13px;
  color: #3A3A3C;
  margin-bottom: 6px;
}
.req {
  color: #FF3B30;
}
.trad-input,
.trad-select {
  width: 100%;
  padding: 10px 12px;
  border: 1px solid #D1D1D6;
  border-radius: 6px;
  font-size: 14px;
  box-sizing: border-box;
  background: #FFFFFF;
  color: #1C1C1E;
  font-family: inherit;
}
.trad-input::placeholder {
  color: #C7C7CC;
}
.trad-input:read-only {
  cursor: default;
}
.trad-select {
  color: #8E8E93;
  cursor: default;
}
.trad-select.sel {
  color: #3A3A3C;
}
.trad-select:disabled {
  opacity: 1;
  color: #8E8E93;
  -webkit-text-fill-color: currentColor;
}
.trad-select.sel:disabled {
  color: #3A3A3C;
  -webkit-text-fill-color: currentColor;
}
.trad-row {
  display: flex;
  gap: 8px;
}
.trad-row .trad-select {
  flex: 1;
}
.date-wrap {
  position: relative;
}
.date-icon {
  position: absolute;
  left: 12px;
  top: 50%;
  transform: translateY(-50%);
  font-size: 14px;
  pointer-events: none;
  line-height: 1;
}
.date-val {
  padding-left: 36px;
  color: #3A3A3C;
}
.trad-submit {
  width: 100%;
  padding: 12px;
  background: #007AFF;
  color: #FFFFFF;
  font-size: 15px;
  font-weight: 500;
  border: none;
  border-radius: 6px;
  cursor: default;
  font-family: inherit;
}

/* ===== 右侧卡片：iOS + Notion 风格 ===== */
.card-modern {
  border-radius: 20px;
  padding: 32px;
  box-shadow: 0 4px 24px rgba(0,0,0,0.06);
  border: 1px solid rgba(0,0,0,0.04);
}
.modern-hero {
  margin-bottom: 0;
}
.modern-title {
  font-size: 22px;
  font-weight: 700;
  color: #1C1C1E;
  letter-spacing: -0.5px;
  margin: 16px 0 8px;
  line-height: 1.2;
}
.modern-sub {
  font-size: 14px;
  color: #8E8E93;
  margin: 0 0 32px;
  line-height: 1.4;
}
.modern-field {
  margin-bottom: 24px;
}
.modern-field.mt-phone {
  margin-bottom: 28px;
}
.modern-field.mt-role {
  margin-bottom: 32px;
}
.modern-label {
  display: block;
  font-size: 13px;
  font-weight: 500;
  color: #3A3A3C;
  letter-spacing: -0.2px;
  margin-bottom: 8px;
}
.modern-label.role-label {
  margin-bottom: 12px;
}
.modern-input {
  width: 100%;
  padding: 14px 16px;
  border: 1.5px solid #E5E5EA;
  border-radius: 12px;
  font-size: 16px;
  color: #1C1C1E;
  background: #FAFAFA;
  box-sizing: border-box;
  outline: none;
  font-family: inherit;
}
.modern-input::placeholder {
  color: #C7C7CC;
}
.modern-input:read-only {
  cursor: default;
}
.helper {
  font-size: 12px;
  color: #8E8E93;
  margin: 6px 0 0 4px;
  line-height: 1.4;
}
.pills {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.pill {
  padding: 8px 16px;
  border-radius: 999px;
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  font-family: inherit;
  display: inline-block;
  line-height: 1.3;
}
.pill.active {
  background: #007AFF;
  color: #FFFFFF;
  border: none;
}
.pill.idle {
  background: #F2F2F7;
  color: #8E8E93;
  border: 1px solid #E5E5EA;
}
.advanced {
  border-top: 1px solid #F2F2F7;
  padding-top: 20px;
  margin-bottom: 28px;
}
.advanced-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  cursor: pointer;
}
.advanced-text {
  font-size: 14px;
  font-weight: 500;
  color: #636366;
}
.chevron {
  color: #636366;
  flex-shrink: 0;
}
.advanced-desc {
  font-size: 12px;
  color: #8E8E93;
  margin: 4px 0 0;
  line-height: 1.4;
}
.modern-primary {
  width: 100%;
  padding: 16px;
  background: #007AFF;
  color: #FFFFFF;
  font-size: 16px;
  font-weight: 600;
  letter-spacing: -0.3px;
  border: none;
  border-radius: 999px;
  box-shadow: 0 2px 8px rgba(0,122,255,0.25);
  cursor: default;
  font-family: inherit;
  display: block;
  line-height: 1.2;
}
.modern-cancel {
  width: 100%;
  padding: 14px;
  background: transparent;
  color: #8E8E93;
  font-size: 15px;
  font-weight: 500;
  border: none;
  border-radius: 999px;
  margin-top: 8px;
  cursor: default;
  font-family: inherit;
  display: block;
  line-height: 1.2;
}
</style>