const { logger } = require('./logger')

/**
 * 账户轮询管理器
 * 负责账户的轮询选择和负载均衡
 */
class AccountRotator {
  constructor() {
    this.accounts = []
    this.currentIndex = 0
    this.lastUsedTimes = new Map() // 记录每个账户的最后使用时间
    this.failureCounts = new Map() // 记录每个账户的失败次数
    this.failureReasons = new Map() // 记录每个账户的失败原因
    this.disabledAccounts = new Set() // 永久禁用的账号列表
    this.maxFailures = 3 // 最大失败次数
    this.cooldownPeriod = 5 * 60 * 1000 // 5分钟冷却期
  }

  /**
   * 设置账户列表
   * @param {Array} accounts - 账户列表
   */
  setAccounts(accounts) {
    if (!Array.isArray(accounts)) {
      logger.error('账户列表必须是数组', 'ACCOUNT')
      throw new Error('账户列表必须是数组')
    }
    
    this.accounts = [...accounts]
    this.currentIndex = 0
    
    // 清理不存在账户的记录
    this._cleanupRecords()
  }

  /**
   * 获取下一个可用账户
   * @returns {Object|null} 可用账户对象或null
   */
  getNextAccount() {
    if (this.accounts.length === 0) {
      return null
    }

    const totalAccounts = this.accounts.length
    let attempts = 0
    let selectedAccount = null

    while (attempts < totalAccounts) {
      const account = this.accounts[this.currentIndex]
      const email = account.email

      // 检查账号是否被禁用
      if (this.isAccountDisabled(email)) {
        attempts++
        this.currentIndex = (this.currentIndex + 1) % totalAccounts
        continue
      }

      const failures = this.failureCounts.get(email) || 0
      const lastUsed = this.lastUsedTimes.get(email) || 0
      const now = Date.now()

      // 检查是否在冷却期
      if (failures >= this.maxFailures && (now - lastUsed) < this.cooldownPeriod) {
        attempts++
        this.currentIndex = (this.currentIndex + 1) % totalAccounts
        continue
      }

      // 找到可用账户
      selectedAccount = account
      this._recordUsage(email)
      this.currentIndex = (this.currentIndex + 1) % totalAccounts
      break
    }

    if (!selectedAccount) {
      logger.warn('所有账号都不可用（禁用或冷却中）', 'ACCOUNT')
    }

    return selectedAccount
  }

  /**
   * 获取指定邮箱的账户令牌
   * @param {string} email - 邮箱地址
   * @returns {string|null} 账户令牌或null
   */
  getTokenByEmail(email) {
    const account = this.accounts.find(acc => acc.email === email)
    if (!account) {
      logger.error(`未找到邮箱为 ${email} 的账户`, 'ACCOUNT')
      return null
    }

    if (!this._isAccountAvailable(account)) {
      logger.warn(`账户 ${email} 当前不可用`, 'ACCOUNT')
      return null
    }

    this._recordUsage(email)
    return account.token
  }

  /**
   * 记录账户使用失败
   * @param {string} email - 邮箱地址
   * @param {string} reason - 失败原因（可选）
   */
  recordFailure(email, reason = null) {
    const currentFailures = this.failureCounts.get(email) || 0
    this.failureCounts.set(email, currentFailures + 1)
    
    // 记录失败原因
    if (reason) {
      const reasons = this.failureReasons.get(email) || []
      reasons.push({
        timestamp: Date.now(),
        reason: reason,
        failureCount: currentFailures + 1
      })
      // 只保留最近10条失败记录
      if (reasons.length > 10) {
        reasons.shift()
      }
      this.failureReasons.set(email, reasons)
    }
    
    if (currentFailures + 1 >= this.maxFailures) {
      logger.warn(`账户 ${email} 失败次数达到上限，将进入冷却期`, 'ACCOUNT')
    }
  }

  /**
   * 重置账户失败计数
   * @param {string} email - 邮箱地址
   */
  resetFailures(email) {
    this.failureCounts.delete(email)
  }

  /**
   * 获取账户统计信息
   * @returns {Object} 统计信息
   */
  getStats() {
    const total = this.accounts.length
    const available = this._getAvailableAccounts().length
    const inCooldown = total - available
    
    const usageStats = {}
    this.accounts.forEach(account => {
      const email = account.email
      usageStats[email] = {
        failures: this.failureCounts.get(email) || 0,
        lastUsed: this.lastUsedTimes.get(email) || null,
        available: this._isAccountAvailable(account)
      }
    })

    return {
      total,
      available,
      inCooldown,
      currentIndex: this.currentIndex,
      usageStats
    }
  }

  /**
   * 获取可用账户列表
   * @private
   */
  _getAvailableAccounts() {
    return this.accounts.filter(account => this._isAccountAvailable(account))
  }

  /**
   * 检查账户是否可用
   * @param {Object} account - 账户对象
   * @returns {boolean} 是否可用
   * @private
   */
  _isAccountAvailable(account) {
    if (!account.token) {
      return false
    }

    const failures = this.failureCounts.get(account.email) || 0
    if (failures >= this.maxFailures) {
      const lastUsed = this.lastUsedTimes.get(account.email)
      if (lastUsed && Date.now() - lastUsed < this.cooldownPeriod) {
        return false // 仍在冷却期
      } else {
        // 冷却期结束，重置失败计数
        this.failureCounts.delete(account.email)
      }
    }

    return true
  }

  /**
   * 选择最少使用的账户
   * @param {Array} accounts - 可用账户列表
   * @returns {Object} 选中的账户
   * @private
   */
  _selectLeastUsedAccount(accounts) {
    if (accounts.length === 1) {
      return accounts[0]
    }

    // 按最后使用时间排序，选择最久未使用的
    return accounts.reduce((least, current) => {
      const leastLastUsed = this.lastUsedTimes.get(least.email) || 0
      const currentLastUsed = this.lastUsedTimes.get(current.email) || 0
      
      return currentLastUsed < leastLastUsed ? current : least
    })
  }

  /**
   * 轮询策略获取令牌
   * @returns {string|null} 账户令牌或null
   * @private
   */
  _getTokenByRoundRobin() {
    if (this.currentIndex >= this.accounts.length) {
      this.currentIndex = 0
    }

    const account = this.accounts[this.currentIndex]
    this.currentIndex++

    if (account && account.token) {
      this._recordUsage(account.email)
      return account.token
    }

    // 如果当前账户无效，尝试下一个
    if (this.currentIndex < this.accounts.length) {
      return this._getTokenByRoundRobin()
    }

    return null
  }

  /**
   * 记录账户使用
   * @param {string} email - 邮箱地址
   * @private
   */
  _recordUsage(email) {
    this.lastUsedTimes.set(email, Date.now())
  }

  /**
   * 清理不存在账户的记录
   * @private
   */
  _cleanupRecords() {
    const currentEmails = new Set(this.accounts.map(acc => acc.email))
    
    // 清理失败计数记录
    for (const email of this.failureCounts.keys()) {
      if (!currentEmails.has(email)) {
        this.failureCounts.delete(email)
      }
    }
    
    // 清理使用时间记录
    for (const email of this.lastUsedTimes.keys()) {
      if (!currentEmails.has(email)) {
        this.lastUsedTimes.delete(email)
      }
    }
  }

  /**
   * 重置所有统计数据
   */
  reset() {
    this.currentIndex = 0
    this.lastUsedTimes.clear()
    this.failureCounts.clear()
  }

  /**
   * 获取所有失败账号列表
   * @returns {Array} 失败账号列表，包含邮箱和失败次数
   */
  getFailedAccounts() {
    const failedAccounts = []
    
    for (const [email, failures] of this.failureCounts.entries()) {
      if (failures > 0) {
        // 查找完整的账户信息
        const account = this.accounts.find(acc => acc.email === email)
        // 获取失败原因记录
        const failureHistory = this.failureReasons.get(email) || []
        
        failedAccounts.push({
          email,
          failures,
          maxFailures: this.maxFailures,
          isLocked: failures >= this.maxFailures,
          lastUsed: this.lastUsedTimes.get(email) || null,
          failureHistory: failureHistory.slice(-5), // 只返回最近5条记录
          accountInfo: account ? {
            expires: account.expires,
            hasToken: !!account.token
          } : null
        })
      }
    }
    
    // 按失败次数降序排列
    return failedAccounts.sort((a, b) => b.failures - a.failures)
  }

  /**
   * 清除指定账号的失败记录
   * @param {string} email - 邮箱地址
   */
  clearFailureHistory(email) {
    this.failureCounts.delete(email)
    this.failureReasons.delete(email)
    logger.info(`已清除账户 ${email} 的失败记录`, 'ACCOUNT')
  }

  /**
   * 清除所有账号的失败记录
   */
  clearAllFailureHistory() {
    this.failureCounts.clear()
    this.failureReasons.clear()
    logger.info('已清除所有账户的失败记录', 'ACCOUNT')
  }

  /**
   * 永久禁用账号
   * @param {string} email - 邮箱地址
   */
  disableAccount(email) {
    this.disabledAccounts.add(email)
    logger.info(`已永久禁用账户 ${email}`, 'ACCOUNT')
  }

  /**
   * 解除账号禁用
   * @param {string} email - 邮箱地址
   */
  enableAccount(email) {
    this.disabledAccounts.delete(email)
    logger.info(`已解除账户 ${email} 的禁用状态`, 'ACCOUNT')
  }

  /**
   * 检查账号是否被禁用
   * @param {string} email - 邮箱地址
   * @returns {boolean} 是否被禁用
   */
  isAccountDisabled(email) {
    return this.disabledAccounts.has(email)
  }

  /**
   * 获取所有禁用的账号列表
   * @returns {Array} 禁用账号列表
   */
  getDisabledAccounts() {
    return Array.from(this.disabledAccounts)
  }
}

module.exports = AccountRotator
