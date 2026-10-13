import { type IProblems } from '@/interfaces/problems.interface'
import { type DetectionRules, type ProblemList, type ProblemsRequest, type SaveResult } from '@/types/problems'

class ProblemsService implements IProblems {
  private instance: IProblems | null = null
  async init (options: IProblems) {
    if (this.instance === null) {
      this.instance = options
    }
  }

  public async problems (request: ProblemsRequest): Promise<[string | null, ProblemList | null]> {
    return await this.instance!.problems(request)
  }

  public async services (timeFrom: string, timeTo: string): Promise<[string | null, string[] | null]> {
    return await this.instance!.services(timeFrom, timeTo)
  }

  public async rules (): Promise<[string | null, DetectionRules | null]> {
    return await this.instance!.rules()
  }

  public async saveRules (rules: DetectionRules, expectedRevision: number): Promise<SaveResult> {
    return await this.instance!.saveRules(rules, expectedRevision)
  }
}

const problemsApi: ProblemsService = new ProblemsService()
export default problemsApi
