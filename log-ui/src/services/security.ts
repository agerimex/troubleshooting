import { store } from '@/services/store'

const Security = {
  // make sure user is authenticated
//   requireToken: function () {
//     if (store.token === '') {
//       router.push('/auth')
//       return false
//     }
//   },

  // create request options and send them back
  requestOptions: function (payload: any, type: string = 'POST') {
    const headers = new Headers()
    headers.append('Content-Type', 'application/json')
    headers.append('Authorization', 'Bearer ' + store.token)

    let res = {}
    if (type === 'GET') {
      res = {
        method: type,
        headers
      }
    } else {
      res = {
        method: type,
        body: JSON.stringify(payload),
        headers
      }
    }
    return res
  },

  // check token
//   checkToken: async function () {
//     if (store.token !== '') {
//       const [error, res] = await authApi.validateToken(store.token)
//       if (error === null && !res.data) {
//         store.token = ''
//         store.user = {}
//         document.cookie = '_site_data=; Path=/; ' + 'SameSite=strict; Secure; ' + 'Expires=Thu, 01 Jan 1970 00:00:01 GMT;'
//       }
//     }
//   }
}

export default Security
