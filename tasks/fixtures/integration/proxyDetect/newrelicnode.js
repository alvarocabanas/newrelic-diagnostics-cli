'use strict'

/**
 * New Relic agent configuration.
 *
 * See lib/config.defaults.js in the agent distribution for a more complete
 * description of configuration variables and their potential values.
 */
// slash comment
exports.config = { // slash comment
  /**
   * Array of application names.
   */
  app_name: ['My Node App'],
  /**
   * Your New Relic license key.
   */
  license_key: 'license-key-val-node',//comment
  proxy: 'http://user:pass@10.0.0.1:8000',
  logging: {
    /**
     * Level at which to log. 'trace' is most useful to New Relic when diagnosing
     * issues with the agent, 'info' and higher will impose the least overhead on
     * production applications.
     */
    level: 'info',
    filepath: 'temp.log'
  }
}
